package main

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestWebSocketHandshakeCommandAndEventAck(t *testing.T) {
	a := fixture(t)
	a.welcomed = false
	var socket *websocket.Conn
	var socketMu sync.Mutex
	ready := make(chan struct{})
	ack := make(chan string, 1)
	event := make(chan string, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws/agents" || r.Header.Get("Authorization") != "Bearer fixture-private-token" {
			t.Error("incorrect endpoint or credential")
			w.WriteHeader(401)
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		socketMu.Lock()
		socket = c
		socketMu.Unlock()
		var hello map[string]any
		if c.ReadJSON(&hello) != nil {
			return
		}
		if hello["type"] != "hello" || hello["v"] != float64(1) || hello["version"] != version {
			t.Error("invalid hello")
		}
		if c.WriteJSON(map[string]any{"type": "welcome", "v": 1, "agentId": "agent_1", "zoneId": "zone_1", "epoch": 2, "leaseMs": 60000}) != nil {
			return
		}
		close(ready)
		for {
			var value map[string]any
			if c.ReadJSON(&value) != nil {
				return
			}
			switch value["type"] {
			case "ack":
				ack <- value["status"].(string)
			case "playback":
				id := value["eventId"].(string)
				if c.WriteJSON(map[string]any{"type": "event-ack", "eventId": id}) == nil {
					event <- id
				}
			}
		}
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer server.Close()
	a.config.ServerURL = server.URL
	if err := atomicJSON(a.data+"/control.json", map[string]bool{"blocked": false}); err != nil {
		t.Fatal(err)
	}
	go a.run(ctx)
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("hello timeout")
	}
	deadline := time.Now().Add(time.Second)
	for {
		a.mu.Lock()
		welcomed := a.welcomed
		a.mu.Unlock()
		if welcomed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("welcome timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
	socketMu.Lock()
	c := socket
	socketMu.Unlock()
	if err := c.WriteJSON(command()); err != nil {
		t.Fatal(err)
	}
	select {
	case status := <-ack:
		if status != "accepted" {
			t.Fatal(status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ack timeout")
	}
	a.record("fixture_play", "started", "")
	select {
	case <-event:
	case <-time.After(3 * time.Second):
		t.Fatal("event timeout")
	}
	deadline = time.Now().Add(time.Second)
	for {
		a.mu.Lock()
		remaining := len(a.journal.Events)
		a.mu.Unlock()
		if remaining == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("event was not removed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	bytes, _ := json.Marshal(a.wire("heartbeat"))
	if !json.Valid(bytes) {
		t.Fatal("invalid heartbeat")
	}
	cancel()
	c.Close()
}
