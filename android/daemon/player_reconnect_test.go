package main

import (
	"context"
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
