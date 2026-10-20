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
	a.maintainPlayer(ctx, func() (playerConnection, error) { return connectPlayer(a.data, a.module) }, time.Second)
}

func (a *Agent) maintainPlayer(ctx context.Context, connect func() (playerConnection, error), interval time.Duration) {
	delay := interval
	wait := func() bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
			return true
		}
	}
	for ctx.Err() == nil {
		player, err := connect()
		if err != nil {
			a.log("播放组件连接失败：" + err.Error())
			a.mu.Lock()
			a.playerError = err.Error()
			a.mu.Unlock()
			if !wait() {
				return
			}
			delay = min(3*interval, delay*2)
			continue
		}
		delay = interval
		a.dispatchMu.Lock()
		a.mu.Lock()
		a.player = player
		a.playerError = ""
		a.playerSeen = time.Now()
		a.playerConnections++
		epoch := a.epoch
		lease := max(0, time.Until(a.leaseUntil).Milliseconds())
		blocked := a.braked
		a.mu.Unlock()
		_ = player.Send(map[string]any{"type": "lease", "epoch": epoch, "leaseMs": lease}, "")
		_ = player.Send(map[string]any{"action": "brake", "blocked": blocked}, "")
		a.dispatchMu.Unlock()
		a.log("原生播放服务已连接")
		a.protectPlayer(player)
		connectionDone := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				player.Close()
			case <-connectionDone:
			}
		}()
		reader := bufio.NewScanner(player)
		reader.Buffer(make([]byte, 4096), 65536)
		for reader.Scan() {
			if ctx.Err() != nil {
				break
			}
			a.playerMessage(reader.Bytes())
		}
		player.Close()
		close(connectionDone)
		a.mu.Lock()
		current := a.playerState.PlaybackID
		if a.player == player {
			a.player = nil
			a.playerState = PlayerState{}
		}
		a.playerError = "原生播放服务已断开"
		a.playerDisconnectedAt = time.Now().Format(time.RFC3339)
		a.mu.Unlock()
		if ctx.Err() == nil {
			reason := "本机连接已关闭"
			if reader.Err() != nil {
				reason = reader.Err().Error()
			}
			a.log("原生播放服务断开：" + reason + "；守护继续运行，1 秒后重新拉起")
			if diagnostics, ok := player.(interface{ DisconnectLog() string }); ok {
				go func() {
					if message := diagnostics.DisconnectLog(); message != "" {
						a.log(message)
					}
				}()
			}
		}
		if current != nil {
			a.record(*current, "failed", "原生播放服务已断开")
		}
		if !wait() {
			return
		}
	}
}

type playerConnection interface {
	Player
	io.Reader
}

// 播放器的本机连接与后端网络分别恢复；断网不暂停原生服务的监控。
func (a *Agent) protectPlayer(player Player) {
	guard, ok := player.(interface{ KeepAlive() error })
	if !ok {
		return
	}
	err := guard.KeepAlive()
	warning := ""
	if err != nil {
		warning = err.Error()
	}
	a.mu.Lock()
	if a.player != player {
		a.mu.Unlock()
		return
	}
	previous := a.playerKeepaliveError
	a.playerKeepaliveError = warning
	a.mu.Unlock()
	if warning != "" && warning != previous {
		a.log("播放器保活设置未完全生效：" + warning + "；仍持续监控和拉起播放器")
	}
	if warning == "" && previous != "" {
		a.log("播放器保活设置已恢复")
	}
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
