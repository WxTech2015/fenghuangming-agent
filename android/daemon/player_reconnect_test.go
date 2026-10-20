package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type reconnectPlayer struct {
	net.Conn
	sent chan map[string]any
}

type guardedPlayer struct{ *reconnectPlayer }

func (p *guardedPlayer) KeepAlive() error { return errors.New("ROM 拒绝优先级调整") }

func TestOfflinePlayerRecoveryDoesNotRequireBackendOrKeepalivePolicy(t *testing.T) {
	a := fixture(t)
	a.player = nil
	a.welcomed = false
	a.leaseUntil = time.Now().Add(-time.Minute)
	a.braked = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	peers := make(chan net.Conn, 2)
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.maintainPlayer(ctx, func() (playerConnection, error) {
			client, peer := net.Pipe()
			peers <- peer
			return &guardedPlayer{&reconnectPlayer{client, make(chan map[string]any, 16)}}, nil
		}, time.Millisecond)
	}()
	for round := 0; round < 2; round++ {
		select {
		case peer := <-peers:
			status, _ := json.Marshal(map[string]any{"type": "status", "state": PlayerState{Ready: true, CpuAwake: true}})
			if _, err := peer.Write(append(status, '\n')); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(time.Second)
			for {
				s := a.state()
				if s["playerConnected"] == true && s["diagnostics"].(map[string]any)["keepaliveError"] != "" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("离线时原生服务没有恢复")
				}
				time.Sleep(time.Millisecond)
			}
			if a.state()["authorized"] != false || a.state()["emergencyStopped"] != true {
				t.Fatal("离线恢复改变了认证或制动状态")
			}
			peer.Close()
		case <-time.After(2 * time.Second):
			t.Fatal("离线时没有重启播放器")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("恢复循环没有退出")
	}
	if a.state()["diagnostics"].(map[string]any)["lastPlayerDisconnect"] == "" {
		t.Fatal("缺少断开时间")
	}
}

func (p *reconnectPlayer) Send(value map[string]any, _ string) error {
	p.sent <- value
	return nil
}

func TestPlayerKeepsRetryingAndPreservesBrake(t *testing.T) {
	a := fixture(t)
	a.player = nil
	a.braked = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sent := make(chan map[string]any, 16)
	peers := make(chan net.Conn, 4)
	done := make(chan struct{})
	var attempts atomic.Int32
	go func() {
		defer close(done)
		a.maintainPlayer(ctx, func() (playerConnection, error) {
			if attempts.Add(1) <= 6 {
				return nil, errors.New("temporary startup failure")
			}
			client, peer := net.Pipe()
			peers <- peer
			return &reconnectPlayer{client, sent}, nil
		}, time.Millisecond)
	}()
	for round := 0; round < 2; round++ {
		select {
		case peer := <-peers:
			select {
			case <-sent: // lease
			case <-time.After(time.Second):
				t.Fatal("missing lease")
			}
			select {
			case value := <-sent:
				if value["action"] != "brake" || value["blocked"] != true {
					t.Fatal("reconnect released the brake")
				}
			case <-time.After(time.Second):
				t.Fatal("missing brake state")
			}
			peer.Close()
		case <-time.After(2 * time.Second):
			t.Fatal("recovery stopped after transient failures")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("player retry loop did not stop")
	}
	if attempts.Load() < 8 {
		t.Fatal("did not reconnect after native disconnect")
	}
}
