package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Player interface {
	Send(map[string]any, string) error
	Close() error
}
type Agent struct {
	mu              sync.Mutex
	wsMu            sync.Mutex
	brakeMu         sync.Mutex
	dispatchMu      sync.Mutex
	cancelMain      context.CancelFunc
	data, module    string
	config          Config
	journal         Journal
	braked          bool
	socket          *websocket.Conn
	welcomed        bool
	agentID, zoneID string
	epoch           int64
	leaseUntil      time.Time
	player          Player
	playerState     PlayerState
	playerSeen      time.Time
	playerError     string
	cancelDownload  context.CancelFunc
	generation      uint64
	current         *Payload
	transfer        *Transfer
	overview        json.RawMessage
	bootID          string
	pending         map[string]chan error
	logs            []string
}

func newAgent(data, module string) (*Agent, error) {
	if err := os.MkdirAll(data, 0700); err != nil {
		return nil, err
	}
	a := &Agent{data: data, module: module, config: Config{CacheMaxMB: 512, Connections: 4}, bootID: newID(), pending: make(map[string]chan error)}
	stalePattern := regexp.MustCompile(`^[a-f0-9]{64}\.[a-f0-9]{32}\.tmp$`)
	if cached, err := os.ReadDir(filepath.Join(data, "cache")); err == nil {
		for _, item := range cached {
			if !item.IsDir() && stalePattern.MatchString(item.Name()) {
				_ = os.Remove(filepath.Join(data, "cache", item.Name()))
			}
		}
	}
	if err := readJSON(filepath.Join(data, "config.json"), &a.config); err != nil && !os.IsNotExist(err) {
		return nil, errors.New("配置文件损坏")
	}
	if err := a.config.validate(); err != nil {
		return nil, err
	}
	var control struct {
		Blocked bool `json:"blocked"`
	}
	if err := readJSON(filepath.Join(data, "control.json"), &control); err != nil {
		a.braked = true
	} else {
		a.braked = control.Blocked
	}
	if err := readJSON(filepath.Join(data, "journal.json"), &a.journal); err != nil && !os.IsNotExist(err) {
		return nil, errors.New("指令记录损坏，停止自动启动")
	}
	return a, nil
}
func (a *Agent) log(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.config.Token != "" {
		message = strings.ReplaceAll(message, a.config.Token, "[REDACTED]")
	}
	line := time.Now().Format(time.RFC3339) + " " + message
	a.logs = append(a.logs, line)
	if len(a.logs) > 200 {
		a.logs = a.logs[len(a.logs)-200:]
	}
	_ = atomicJSON(filepath.Join(a.data, "logs.json"), a.logs)
}
func (a *Agent) readyLocked() bool {
	return a.player != nil && a.playerState.Ready && time.Since(a.playerSeen) < 6*time.Second && !a.braked && a.config.Token != "" && len(a.journal.Events) < 512
}
func (a *Agent) state() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	var current any
	if a.current != nil {
		current = map[string]any{"title": a.current.Title, "artists": a.current.Artists, "playbackId": a.current.PlaybackID}
	}
	return map[string]any{"version": version, "endpoint": a.config.ServerURL, "hasToken": a.config.Token != "", "authorized": a.welcomed, "serverConnected": a.socket != nil, "playerConnected": a.player != nil && time.Since(a.playerSeen) < 6*time.Second, "playerReady": a.readyLocked(), "playerError": a.playerError, "emergencyStopped": a.braked, "player": a.playerState, "current": current, "transfer": a.transfer, "overview": a.overview, "cacheMaxMb": a.config.CacheMaxMB, "connections": a.config.Connections}
}
func (a *Agent) wire(kind string) map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.playerState
	value := map[string]any{"type": kind, "ready": a.readyLocked(), "playbackId": s.PlaybackID, "started": s.Started, "position": s.Position, "paused": s.Paused, "durationSeconds": s.Duration, "seekable": s.Seekable}
	if kind == "hello" {
		value["v"] = 1
		value["version"] = version
		value["bootId"] = a.bootID
		value["capabilities"] = []string{"seek"}
	}
	return value
}
func (a *Agent) send(value any) {
	a.mu.Lock()
	socket := a.socket
	a.mu.Unlock()
	if socket == nil {
		return
	}
	a.wsMu.Lock()
	defer a.wsMu.Unlock()
	_ = socket.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = socket.WriteJSON(value)
}
func (a *Agent) record(playback, state, reason string) {
	a.mu.Lock()
	event := Event{"playback", newID(), playback, state, reason}
	a.journal.Events = append(a.journal.Events, event)
	if state != "started" && a.current != nil && a.current.PlaybackID == playback {
		a.current = nil
	}
	err := atomicJSON(filepath.Join(a.data, "journal.json"), a.journal)
	a.mu.Unlock()
	if err != nil {
		_ = a.setBrake(true)
		a.log("播放事件持久化失败，保持制动")
		return
	}
	a.send(event)
	a.log("播放事件：" + state)
}
func (a *Agent) run(ctx context.Context) {
	go a.tick(ctx)
	go a.playerLoop(ctx)
	delay := time.Second
	for ctx.Err() == nil {
		a.mu.Lock()
		config := a.config
		a.mu.Unlock()
		if config.Token == "" {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}
		u, _ := url.Parse(config.ServerURL + "/ws/agents")
		if u.Scheme == "https" {
			u.Scheme = "wss"
		} else {
			u.Scheme = "ws"
		}
		header := http.Header{}
		header.Set("Authorization", "Bearer "+config.Token)
		dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
		socket, response, err := dialer.DialContext(ctx, u.String(), header)
		if response != nil && err != nil {
			response.Body.Close()
		}
		if err != nil {
			a.log("后端连接失败")
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			delay = min(30*time.Second, delay*2)
			continue
		}
		delay = time.Second
		socket.SetReadLimit(64000)
		_ = socket.SetReadDeadline(time.Now().Add(45 * time.Second))
		a.mu.Lock()
		a.socket = socket
		a.mu.Unlock()
		a.send(a.wire("hello"))
		a.log("后端 WebSocket 已连接")
		for {
			_, bytes, err := socket.ReadMessage()
			if err != nil {
				break
			}
			_ = socket.SetReadDeadline(time.Now().Add(45 * time.Second))
			if err = a.frame(bytes); err != nil {
				a.log("后端协议被拒绝")
				break
			}
		}
		socket.Close()
		a.mu.Lock()
		if a.socket == socket {
			a.socket = nil
			a.welcomed = false
		}
		if a.cancelDownload != nil {
			a.cancelDownload()
		}
		a.mu.Unlock()
	}
}
func (a *Agent) frame(bytes []byte) error {
	var value struct {
		Type    string  `json:"type"`
		V       int     `json:"v"`
		AgentID string  `json:"agentId"`
		ZoneID  string  `json:"zoneId"`
		Epoch   int64   `json:"epoch"`
		LeaseMS float64 `json:"leaseMs"`
		EventID string  `json:"eventId"`
	}
	if json.Unmarshal(bytes, &value) != nil {
		return errors.New("invalid JSON")
	}
	switch value.Type {
	case "welcome", "lease":
		if value.Epoch < 1 || value.LeaseMS <= 0 || math.IsNaN(value.LeaseMS) || math.IsInf(value.LeaseMS, 0) {
			return errors.New("invalid lease")
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if value.Type == "welcome" {
			if value.V != 1 || value.AgentID == "" || value.ZoneID == "" {
				return errors.New("invalid identity")
			}
			a.agentID = value.AgentID
			a.zoneID = value.ZoneID
			a.epoch = value.Epoch
			a.welcomed = true
		}
		if value.Epoch == a.epoch {
			a.leaseUntil = time.Now().Add(time.Duration(min(60000.0, value.LeaseMS)) * time.Millisecond)
		}
		return nil
	case "overview":
		a.mu.Lock()
		a.overview = append(json.RawMessage(nil), bytes...)
		a.mu.Unlock()
		return nil
	case "event-ack":
		a.mu.Lock()
		defer a.mu.Unlock()
		events := a.journal.Events[:0]
		for _, e := range a.journal.Events {
			if e.EventID != value.EventID {
				events = append(events, e)
			}
		}
		a.journal.Events = events
		return atomicJSON(filepath.Join(a.data, "journal.json"), a.journal)
	}
	var command Command
	if err := json.Unmarshal(bytes, &command); err != nil {
		return err
	}
	if command.Kind != "command" {
		return nil
	}
	status, reason := a.accept(command)
	ack := map[string]any{"type": "ack", "commandId": command.ID, "status": status}
	if reason != "" {
		ack["error"] = reason
	}
	a.send(ack)
	return nil
}
func (a *Agent) accept(c Command) (string, string) {
	a.mu.Lock()
	expires, err := time.Parse(time.RFC3339Nano, c.ExpiresAt)
	if err != nil || c.V != 1 || c.ID == "" || len(c.ID) > 200 || !a.welcomed || c.AgentID != a.agentID || c.ZoneID != a.zoneID || c.Epoch != a.epoch || !expires.After(time.Now()) || time.Now().After(a.leaseUntil) {
		a.mu.Unlock()
		return "rejected", "设备身份、租约或指令期限不正确"
	}
	for _, id := range a.journal.Commands {
		if id == c.ID {
			a.mu.Unlock()
			return "duplicate", ""
		}
	}
	p := c.Payload
	if !a.readyLocked() && p.Action != "stop" {
		a.mu.Unlock()
		return "rejected", "播放器未就绪或本机已制动"
	}
	switch p.Action {
	case "start":
		if a.current != nil || a.playerState.PlaybackID != nil {
			a.mu.Unlock()
			return "rejected", "本机已有播放任务"
		}
		if p.PlaybackID == "" || p.AssetID == "" || p.Ticket == "" || !hashPattern.MatchString(p.SHA256) || p.Bytes < 1 || p.Bytes > 200*1024*1024 || p.Volume < 0 || p.Volume > 100 {
			a.mu.Unlock()
			return "rejected", "播放参数无效"
		}
	case "seek":
		if a.playerState.PlaybackID == nil || *a.playerState.PlaybackID != p.PlaybackID || !a.playerState.Started || !a.playerState.Seekable || p.Position < 0 || math.IsNaN(p.Position) || math.IsInf(p.Position, 0) || math.Floor(p.Position) != p.Position || p.Position > max(0, a.playerState.Duration-1) {
			a.mu.Unlock()
			return "rejected", "当前任务不可跳转"
		}
	case "pause", "resume", "stop":
		if p.PlaybackID == "" {
			a.mu.Unlock()
			return "rejected", "缺少播放任务"
		}
		if p.Action != "stop" && (a.playerState.PlaybackID == nil || *a.playerState.PlaybackID != p.PlaybackID) {
			a.mu.Unlock()
			return "rejected", "当前任务已改变"
		}
	case "volume":
		if p.Volume < 0 || p.Volume > 100 {
			a.mu.Unlock()
			return "rejected", "音量无效"
		}
	default:
		a.mu.Unlock()
		return "rejected", "未知播放指令"
	}
	a.journal.Commands = append(a.journal.Commands, c.ID)
	if len(a.journal.Commands) > 500 {
		a.journal.Commands = a.journal.Commands[len(a.journal.Commands)-500:]
	}
	if atomicJSON(filepath.Join(a.data, "journal.json"), a.journal) != nil {
		a.mu.Unlock()
		return "rejected", "无法保存指令"
	}
	if p.Action == "start" {
		if a.cancelDownload != nil {
			a.cancelDownload()
		}
		a.generation++
		generation := a.generation
		ctx, cancel := context.WithCancel(context.Background())
		a.cancelDownload = cancel
		a.current = &p
		config := a.config
		a.mu.Unlock()
		go a.start(ctx, c, generation, config)
		return "accepted", ""
	}
	if p.Action == "stop" && a.current != nil && a.current.PlaybackID == p.PlaybackID {
		if a.cancelDownload != nil {
			a.cancelDownload()
		}
		a.generation++
		a.transfer = nil
	}
	pendingStop := p.Action == "stop" && a.current != nil && a.current.PlaybackID == p.PlaybackID && (a.playerState.PlaybackID == nil || *a.playerState.PlaybackID != p.PlaybackID)
	a.mu.Unlock()
	if pendingStop {
		a.record(p.PlaybackID, "stopped", "后端停止下载中的歌曲")
	}
	go func() {
		value := payloadMap(p)
		if err := a.playerCommand(value, ""); err != nil && p.Action != "stop" {
			a.log("播放器控制失败")
		}
	}()
	return "accepted", ""
}
func payloadMap(p Payload) map[string]any {
	b, _ := json.Marshal(p)
	value := map[string]any{}
	_ = json.Unmarshal(b, &value)
	delete(value, "ticket")
	delete(value, "assetId")
	return value
}
func (a *Agent) start(ctx context.Context, c Command, generation uint64, config Config) {
	p := c.Payload
	a.log(fmt.Sprintf("开始准备音频：%d 字节", p.Bytes))
	if p.Bytes > config.CacheMaxMB*1024*1024 {
		a.record(p.PlaybackID, "failed", "歌曲大小超过本机缓存上限")
		return
	}
	a.evict("", max(0, config.CacheMaxMB-(p.Bytes+1024*1024-1)/(1024*1024)))
	u, _ := url.Parse(config.ServerURL + "/api/v1/media/" + url.PathEscape(p.AssetID))
	query := u.Query()
	query.Set("ticket", p.Ticket)
	u.RawQuery = query.Encode()
	file, err := downloadAudio(ctx, filepath.Join(a.data, "cache"), u.String(), config.Token, p, config.Connections, func(value Transfer) {
		a.mu.Lock()
		if a.generation == generation {
			a.transfer = &value
		}
		a.mu.Unlock()
	})
	a.mu.Lock()
	valid := err == nil && ctx.Err() == nil && a.generation == generation && a.welcomed && time.Now().Before(a.leaseUntil) && !a.braked && a.epoch == c.Epoch
	if a.generation == generation {
		a.transfer = nil
	}
	stillCurrent := a.current != nil && a.current.PlaybackID == p.PlaybackID
	a.mu.Unlock()
	if !valid {
		a.log("音频下载失败或任务已取消")
		if stillCurrent {
			a.record(p.PlaybackID, "failed", "下载失败或播放任务已取消")
		}
		return
	}
	value := payloadMap(p)
	value["generation"] = generation
	value["epoch"] = c.Epoch
	if err = a.playerCommand(value, file); err != nil {
		a.record(p.PlaybackID, "failed", "原生播放器未接受歌曲")
		return
	}
	a.log("音频校验完成，已交付原生播放器")
	a.evict(file, config.CacheMaxMB)
}
func (a *Agent) evict(protected string, limit int64) {
	items, _ := os.ReadDir(filepath.Join(a.data, "cache"))
	type cached struct {
		name string
		size int64
		at   time.Time
	}
	var list []cached
	var total int64
	for _, item := range items {
		if !hashPattern.MatchString(strings.TrimSuffix(item.Name(), ".audio")) || !strings.HasSuffix(item.Name(), ".audio") {
			continue
		}
		info, e := item.Info()
		if e == nil {
			total += info.Size()
			list = append(list, cached{item.Name(), info.Size(), info.ModTime()})
		}
	}
	for total > limit*1024*1024 {
		oldest := -1
		for i, item := range list {
			if item.size > 0 && filepath.Join(a.data, "cache", item.name) != protected && (oldest < 0 || item.at.Before(list[oldest].at)) {
				oldest = i
			}
		}
		if oldest < 0 {
			return
		}
		item := &list[oldest]
		if os.Remove(filepath.Join(a.data, "cache", item.name)) == nil {
			total -= item.size
		}
		item.size = 0
	}
}
func (a *Agent) playerCommand(value map[string]any, file string) error {
	// 只串行化校验与发送，等待播放器回执时不占用发送锁。
	a.dispatchMu.Lock()
	a.mu.Lock()
	player := a.player
	if value["action"] == "start" && (value["generation"] != a.generation || value["epoch"] != a.epoch || a.current == nil || a.current.PlaybackID != value["playbackId"] || a.braked || !a.welcomed || !time.Now().Before(a.leaseUntil)) {
		a.mu.Unlock()
		a.dispatchMu.Unlock()
		return errors.New("播放任务已取消")
	}
	if player == nil {
		a.mu.Unlock()
		a.dispatchMu.Unlock()
		return errors.New("原生播放器未连接")
	}
	id := newID()
	result := make(chan error, 1)
	a.pending[id] = result
	value["epoch"] = a.epoch
	lease := map[string]any{"type": "lease", "epoch": a.epoch, "leaseMs": max(0, time.Until(a.leaseUntil).Milliseconds())}
	a.mu.Unlock()
	defer func() { a.mu.Lock(); delete(a.pending, id); a.mu.Unlock() }()
	value["requestId"] = id
	delete(value, "generation")
	var sendError error
	if value["action"] != "brake" {
		sendError = player.Send(lease, "")
	}
	if sendError == nil {
		sendError = player.Send(value, file)
	}
	a.dispatchMu.Unlock()
	if sendError != nil {
		return sendError
	}
	timeout := 8 * time.Second
	if value["action"] == "brake" && value["blocked"] == true {
		timeout = 250 * time.Millisecond
	}
	select {
	case err := <-result:
		return err
	case <-time.After(timeout):
		return errors.New("原生播放器响应超时")
	}
}
func (a *Agent) setBrake(blocked bool) error {
	a.brakeMu.Lock()
	defer a.brakeMu.Unlock()
	if !blocked {
		if err := atomicJSON(filepath.Join(a.data, "control.json"), map[string]bool{"blocked": false}); err != nil {
			return err
		}
	}
	a.mu.Lock()
	a.braked = blocked
	a.generation++
	if a.cancelDownload != nil {
		a.cancelDownload()
	}
	player := a.player
	a.mu.Unlock()
	var persistError error
	if blocked {
		persistError = atomicJSON(filepath.Join(a.data, "control.json"), map[string]bool{"blocked": true})
	}
	if player != nil {
		result := make(chan error, 1)
		go func() { result <- a.playerCommand(map[string]any{"action": "brake", "blocked": blocked}, "") }()
		select {
		case err := <-result:
			if err != nil && blocked {
				_ = player.Close()
				_ = stopPlayer()
			}
		case <-time.After(250 * time.Millisecond):
			if blocked {
				_ = player.Close()
				_ = stopPlayer()
			}
		}
	}
	a.send(a.wire("heartbeat"))
	if blocked {
		a.log("本机紧急制动已锁定")
	} else {
		a.log("本机制动已解除")
	}
	return persistError
}
func (a *Agent) tick(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	counter := 0
	for {
		select {
		case <-ctx.Done():
			a.mu.Lock()
			if a.socket != nil {
				a.socket.Close()
			}
			if a.player != nil {
				a.player.Close()
			}
			a.mu.Unlock()
			return
		case <-ticker.C:
		}
		a.mu.Lock()
		player := a.player
		stale := player != nil && time.Since(a.playerSeen) >= 6*time.Second
		lease := time.Until(a.leaseUntil)
		epoch := a.epoch
		if lease <= 0 && a.cancelDownload != nil {
			a.cancelDownload()
		}
		events := append([]Event(nil), a.journal.Events...)
		a.mu.Unlock()
		if a.module != "" {
			for _, flag := range []string{filepath.Join(a.module, "disable"), filepath.Join(a.module, "remove"), filepath.Join(a.data, "service.stopped")} {
				if _, err := os.Stat(flag); err == nil && a.cancelMain != nil {
					a.cancelMain()
					break
				}
			}
		}
		if stale && player != nil {
			_ = player.Close()
		}
		if player != nil {
			_ = player.Send(map[string]any{"type": "lease", "epoch": epoch, "leaseMs": max(0, lease.Milliseconds())}, "")
		}
		counter++
		if counter%5 == 0 {
			a.send(a.wire("heartbeat"))
			for _, e := range events {
				a.send(e)
			}
		}
		var control struct {
			Blocked bool `json:"blocked"`
		}
		if readJSON(filepath.Join(a.data, "control.json"), &control) != nil {
			control.Blocked = true
		}
		{
			a.mu.Lock()
			different := control.Blocked != a.braked
			a.mu.Unlock()
			if different {
				_ = a.setBrake(control.Blocked)
			}
		}
	}
}
func (a *Agent) playerMessage(bytes []byte) {
	var value struct {
		Type, RequestID, Error     string
		OK                         bool
		State                      PlayerState
		PlaybackID, EventID, Event string
	}
	if json.Unmarshal(bytes, &value) != nil {
		return
	}
	switch value.Type {
	case "status":
		s := value.State
		if math.IsNaN(s.Position) || math.IsInf(s.Position, 0) || s.Position < 0 || s.Duration < 0 || s.Duration > 86400 {
			return
		}
		a.mu.Lock()
		a.playerState = s
		a.playerSeen = time.Now()
		if s.Error != "" {
			a.playerError = s.Error
		}
		a.mu.Unlock()
	case "result":
		a.mu.Lock()
		channel := a.pending[value.RequestID]
		a.mu.Unlock()
		if channel != nil {
			var err error
			if !value.OK {
				err = errors.New("播放器拒绝指令")
			}
			select {
			case channel <- err:
			default:
			}
		}
	case "event":
		if value.PlaybackID != "" && (value.Event == "started" || value.Event == "ended" || value.Event == "stopped" || value.Event == "failed") {
			a.mu.Lock()
			token := a.config.Token
			a.mu.Unlock()
			reason := value.Error
			if token != "" {
				reason = strings.ReplaceAll(reason, token, "[REDACTED]")
			}
			if len(reason) > 500 {
				reason = reason[:500]
			}
			a.record(value.PlaybackID, value.Event, reason)
		}
	}
}
func (a *Agent) control(input map[string]json.RawMessage) (any, error) {
	var action string
	_ = json.Unmarshal(input["action"], &action)
	switch action {
	case "state":
		return a.state(), nil
	case "logs":
		a.mu.Lock()
		recent, token := append([]string(nil), a.logs...), a.config.Token
		a.mu.Unlock()
		return map[string]any{"lines": diagnosticLogs(a.data, token, recent)}, nil
	case "brake", "release":
		err := a.setBrake(action == "brake")
		return map[string]bool{"ok": err == nil}, err
	case "configure":
		var next Config
		if err := json.Unmarshal(input["config"], &next); err != nil {
			return nil, err
		}
		a.mu.Lock()
		if next.Token == "" {
			next.Token = a.config.Token
		}
		a.mu.Unlock()
		if err := next.validate(); err != nil {
			return nil, err
		}
		if next.Token == "" {
			return nil, errors.New("请填写设备凭证")
		}
		if err := a.setBrake(true); err != nil {
			return nil, err
		}
		if err := atomicJSON(filepath.Join(a.data, "config.json"), next); err != nil {
			return nil, err
		}
		a.mu.Lock()
		a.config = next
		a.overview = nil
		a.welcomed = false
		socket := a.socket
		a.mu.Unlock()
		if socket != nil {
			socket.Close()
		}
		return map[string]bool{"ok": true}, nil
	case "activate":
		return map[string]bool{"ok": true}, activatePlayer()
	case "pause", "resume", "stop", "seek", "volume":
		var value float64
		if action == "seek" || action == "volume" {
			if json.Unmarshal(input["value"], &value) != nil {
				return nil, errors.New("缺少有效的控制数值")
			}
		}
		a.mu.Lock()
		s := a.playerState
		var requestedID string
		_ = json.Unmarshal(input["playbackId"], &requestedID)
		if action != "volume" && (requestedID == "" || (s.PlaybackID == nil || *s.PlaybackID != requestedID) && (action != "stop" || a.current == nil || a.current.PlaybackID != requestedID)) {
			a.mu.Unlock()
			return nil, errors.New("播放任务已改变，请刷新后重试")
		}
		pendingStop := action == "stop" && a.current != nil && a.current.PlaybackID == requestedID && (s.PlaybackID == nil || *s.PlaybackID != requestedID)
		if action == "stop" {
			a.generation++
			if a.cancelDownload != nil {
				a.cancelDownload()
				a.cancelDownload = nil
			}
			a.transfer = nil
		}
		blocked := a.braked
		epoch := a.epoch
		lease := time.Now().Before(a.leaseUntil)
		a.mu.Unlock()
		if blocked && action != "stop" {
			return nil, errors.New("请先解除制动")
		}
		if !lease && action != "stop" {
			return nil, errors.New("后端连接租约已过期")
		}
		payload := map[string]any{"action": action, "epoch": epoch}
		if requestedID != "" {
			payload["playbackId"] = requestedID
		}
		if pendingStop {
			a.record(requestedID, "stopped", "本机停止下载中的歌曲")
		}
		if action == "seek" {
			if math.Floor(value) != value || value < 0 || !s.Started || !s.Seekable || value > max(0, s.Duration-1) {
				return nil, errors.New("跳转位置无效")
			}
			payload["position"] = value
		}
		if action == "volume" {
			if math.Floor(value) != value || value < 0 || value > 100 {
				return nil, errors.New("音量无效")
			}
			payload["volume"] = value
		}
		return map[string]bool{"ok": true}, a.playerCommand(payload, "")
	default:
		return nil, fmt.Errorf("未知管理操作")
	}
}
