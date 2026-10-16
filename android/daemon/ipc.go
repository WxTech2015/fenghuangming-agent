package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

func (a *Agent) playerLoop(ctx context.Context) {
	delay := time.Second
	for ctx.Err() == nil {
		a.mu.Lock()
		blocked := a.braked
		locked := a.playerLocked
		a.mu.Unlock()
		if blocked || locked {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}
		player, err := connectPlayer(a.module)
		if err != nil {
			a.mu.Lock()
			a.playerError = err.Error()
			a.playerFailures++
			if a.playerFailures >= 5 {
				a.playerLocked = true
				a.playerError = "播放组件连续启动失败，已停止自动恢复，请点击启动播放器"
			}
			a.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			delay = min(30*time.Second, delay*2)
			continue
		}
		delay = time.Second
		connectedAt := time.Now()
		a.mu.Lock()
		a.player = player
		a.playerError = ""
		a.playerSeen = time.Now()
		epoch := a.epoch
		lease := max(0, time.Until(a.leaseUntil).Milliseconds())
		a.mu.Unlock()
		_ = player.Send(map[string]any{"type": "lease", "epoch": epoch, "leaseMs": lease}, "")
		_ = player.Send(map[string]any{"action": "brake", "blocked": false}, "")
		reader := bufio.NewScanner(player)
		reader.Buffer(make([]byte, 4096), 65536)
		for reader.Scan() {
			if ctx.Err() != nil {
				break
			}
			a.playerMessage(reader.Bytes())
		}
		player.Close()
		a.mu.Lock()
		current := a.playerState.PlaybackID
		if a.player == player {
			a.player = nil
			a.playerState = PlayerState{}
		}
		a.playerError = "原生播放服务已断开"
		if time.Since(connectedAt) >= 10*time.Minute {
			a.playerFailures = 0
		}
		a.playerFailures++
		if a.playerFailures >= 5 {
			a.playerLocked = true
			a.playerError = "播放组件反复退出，已停止自动恢复"
		}
		a.mu.Unlock()
		if current != nil {
			a.record(*current, "failed", "原生播放服务已断开")
		}
	}
}

type playerConnection interface {
	Player
	io.Reader
}

func serveControl(ctx context.Context, a *Agent) error {
	listener, err := listenControl(filepath.Join(a.data, "control.sock"))
	if err != nil {
		return err
	}
	defer listener.Close()
	go a.run(ctx)
	go func() { <-ctx.Done(); listener.Close() }()
	for {
		connection, err := listener.Accept()
		if err != nil {
			return ctx.Err()
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(12 * time.Second))
			if !controlPeer(c) {
				return
			}
			decoder := json.NewDecoder(io.LimitReader(c, 32769))
			var input map[string]json.RawMessage
			if decoder.Decode(&input) != nil {
				return
			}
			result, err := a.control(input)
			if err != nil {
				_ = json.NewEncoder(c).Encode(map[string]any{"ok": false, "error": err.Error()})
			} else {
				_ = json.NewEncoder(c).Encode(result)
			}
		}(connection)
	}
}
func controlCLI(data, encoded string) error {
	if len(encoded) > 32768 {
		return errors.New("管理请求过大")
	}
	bytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return errors.New("管理请求编码无效")
	}
	var request map[string]json.RawMessage
	if json.Unmarshal(bytes, &request) != nil {
		return errors.New("管理请求无效")
	}
	connection, err := net.DialTimeout("unix", filepath.Join(data, "control.sock"), 2*time.Second)
	if err != nil {
		var action string
		_ = json.Unmarshal(request["action"], &action)
		if action == "brake" {
			if e := atomicJSON(filepath.Join(data, "control.json"), map[string]bool{"blocked": true}); e != nil {
				return e
			}
			_ = stopPlayer()
			fmt.Println(`{"ok":true,"fallback":true}`)
			return nil
		}
		if action == "state" {
			blocked := true
			var control struct {
				Blocked bool `json:"blocked"`
			}
			if readJSON(filepath.Join(data, "control.json"), &control) == nil {
				blocked = control.Blocked
			}
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"authorized": false, "playerConnected": false, "playerReady": false, "daemonOffline": true, "emergencyStopped": blocked})
			return nil
		}
		if action == "logs" {
			var config Config
			_ = readJSON(filepath.Join(data, "config.json"), &config)
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"lines": diagnosticLogs(data, config.Token, nil)})
			return nil
		}
		return errors.New("守护进程未运行")
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(12 * time.Second))
	if _, err = connection.Write(append(bytes, '\n')); err != nil {
		return err
	}
	response, err := io.ReadAll(io.LimitReader(connection, 131072))
	if err != nil {
		return err
	}
	fmt.Print(string(response))
	return nil
}
