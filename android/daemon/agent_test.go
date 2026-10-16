package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakePlayer struct{ agent *Agent }

func (p *fakePlayer) Send(value map[string]any, _ string) error {
	if id, ok := value["requestId"].(string); ok {
		bytes, _ := json.Marshal(map[string]any{"type": "result", "requestId": id, "ok": true})
		p.agent.playerMessage(bytes)
	}
	return nil
}
func (p *fakePlayer) Close() error { return nil }
func fixture(t *testing.T) *Agent {
	t.Helper()
	a, err := newAgent(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	a.config = Config{"http://127.0.0.1", "fixture-private-token", 512, 4}
	a.braked = false
	a.player = &fakePlayer{a}
	a.playerState = PlayerState{Ready: true}
	a.playerSeen = time.Now()
	a.welcomed = true
	a.agentID = "agent_1"
	a.zoneID = "zone_1"
	a.epoch = 2
	a.leaseUntil = time.Now().Add(time.Minute)
	return a
}
func command() Command {
	return Command{V: 1, Kind: "command", ID: "cmd_1", AgentID: "agent_1", ZoneID: "zone_1", Epoch: 2, ExpiresAt: time.Now().Add(time.Minute).Format(time.RFC3339Nano), Payload: Payload{Action: "volume", Volume: 50}}
}
func TestCommandIdentityAndDeduplication(t *testing.T) {
	a := fixture(t)
	c := command()
	status, _ := a.accept(c)
	if status != "accepted" {
		t.Fatal(status)
	}
	status, _ = a.accept(c)
	if status != "duplicate" {
		t.Fatal(status)
	}
	c.ID = "cmd_2"
	c.Epoch = 1
	status, _ = a.accept(c)
	if status != "rejected" {
		t.Fatal(status)
	}
	c.Epoch = 2
	c.ExpiresAt = time.Now().Add(-time.Second).Format(time.RFC3339Nano)
	status, _ = a.accept(c)
	if status != "rejected" {
		t.Fatal(status)
	}
	time.Sleep(20 * time.Millisecond)
}
func TestBrakeSurvivesRestartAndDoesNotLeakCredentials(t *testing.T) {
	a := fixture(t)
	a.current = &Payload{Title: "title", Ticket: "fixture-ticket-private", PlaybackID: "play_1"}
	if err := a.setBrake(true); err != nil {
		t.Fatal(err)
	}
	bytes, _ := json.Marshal(a.state())
	if strings.Contains(string(bytes), a.config.Token) || strings.Contains(string(bytes), a.current.Ticket) {
		t.Fatal("state leaked credentials")
	}
	b, err := newAgent(a.data, "")
	if err != nil || !b.braked {
		t.Fatal("brake not persisted", err)
	}
	if a.readyLocked() {
		t.Fatal("braked agent is ready")
	}
}
func TestSeekTaskAndIntegerBounds(t *testing.T) {
	a := fixture(t)
	id := "play_1"
	a.playerState = PlayerState{Ready: true, PlaybackID: &id, Started: true, Seekable: true, Duration: 100}
	for _, position := range []float64{-1, 1.5, 100} {
		c := command()
		c.Payload = Payload{Action: "seek", PlaybackID: id, Position: position}
		if status, _ := a.accept(c); status != "rejected" {
			t.Fatalf("accepted %v", position)
		}
	}
	c := command()
	c.Payload = Payload{Action: "seek", PlaybackID: "old", Position: 10}
	if status, _ := a.accept(c); status != "rejected" {
		t.Fatal("accepted old task")
	}
}
func TestConfigRejectsPathsAndCredentialsInURL(t *testing.T) {
	for _, url := range []string{"ftp://example.com", "http://x:y@example.com", "http://example.com/api", "http://example.com?token=a"} {
		c := Config{url, "fixture-token", 512, 4}
		if c.validate() == nil {
			t.Fatal(url)
		}
	}
}
func TestCorruptControlFailsClosed(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "control.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := newAgent(directory, "")
	if err != nil || !a.braked {
		t.Fatal("corrupt brake opened", err)
	}
}
func TestDownloadRangeAndFallbackIntegrity(t *testing.T) {
	for _, wrongRange := range []bool{false, true} {
		t.Run(fmt.Sprint(wrongRange), func(t *testing.T) {
			data := []byte(strings.Repeat("music", 600000))
			digest := sha256.Sum256(data)
			var ranged, full atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture-token" {
					t.Error("missing authorization")
				}
				header := r.Header.Get("Range")
				if header != "" {
					ranged.Add(1)
					var start, end int
					fmt.Sscanf(header, "bytes=%d-%d", &start, &end)
					if wrongRange {
						w.WriteHeader(200)
						_, _ = w.Write(data)
						return
					}
					w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
					w.WriteHeader(206)
					_, _ = w.Write(data[start : end+1])
				} else {
					full.Add(1)
					_, _ = w.Write(data)
				}
			}))
			defer server.Close()
			p := Payload{SHA256: hex.EncodeToString(digest[:]), Bytes: int64(len(data))}
			file, err := downloadAudio(context.Background(), t.TempDir(), server.URL, "fixture-token", p, 4, func(Transfer) {})
			if err != nil {
				t.Fatal(err)
			}
			actual, err := hashFile(file)
			if err != nil || actual != p.SHA256 {
				t.Fatal("hash mismatch", err)
			}
			if ranged.Load() < 1 {
				t.Fatal("no range")
			}
			if wrongRange && full.Load() != 1 {
				t.Fatal("missing fallback")
			}
		})
	}
}
func TestDownloadCorruptAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("wrong")) }))
	defer server.Close()
	p := Payload{SHA256: strings.Repeat("0", 64), Bytes: 5}
	if _, err := downloadAudio(context.Background(), t.TempDir(), server.URL, "fixture", p, 1, func(Transfer) {}); err == nil {
		t.Fatal("accepted corrupt audio")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := downloadAudio(ctx, t.TempDir(), server.URL, "fixture", p, 1, func(Transfer) {}); err == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestLeaseEpochAndCap(t *testing.T) {
	a := fixture(t)
	if err := a.frame([]byte(`{"type":"lease","epoch":2,"leaseMs":900000}`)); err != nil {
		t.Fatal(err)
	}
	if time.Until(a.leaseUntil) > 60*time.Second {
		t.Fatal("unbounded lease")
	}
	old := a.leaseUntil
	if err := a.frame([]byte(`{"type":"lease","epoch":1,"leaseMs":1}`)); err != nil {
		t.Fatal(err)
	}
	if a.leaseUntil != old {
		t.Fatal("old epoch changed lease")
	}
}

func localRequest(action, playback string, value int) map[string]json.RawMessage {
	bytes, _ := json.Marshal(map[string]any{"action": action, "playbackId": playback, "value": value})
	var request map[string]json.RawMessage
	_ = json.Unmarshal(bytes, &request)
	return request
}

func TestLocalControlsRejectStalePlayback(t *testing.T) {
	a := fixture(t)
	id := "new_play"
	a.playerState = PlayerState{Ready: true, PlaybackID: &id, Started: true, Seekable: true, Duration: 180}
	for _, action := range []string{"pause", "resume", "seek", "stop"} {
		if _, err := a.control(localRequest(action, "old_play", 69)); err == nil {
			t.Fatal("accepted stale local control", action)
		}
	}
	if _, err := a.control(localRequest("seek", id, 69)); err != nil {
		t.Fatal(err)
	}
}

func TestStopCancelsPendingDownloadBeforePlayback(t *testing.T) {
	a := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelDownload = cancel
	a.current = &Payload{Action: "start", PlaybackID: "pending_play"}
	a.transfer = &Transfer{}
	generation := a.generation
	if _, err := a.control(localRequest("stop", "pending_play", 0)); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil || a.transfer != nil || a.current != nil || generation == a.generation {
		t.Fatal("pending download survived stop")
	}
	if err := a.playerCommand(map[string]any{"action": "start", "playbackId": "pending_play", "generation": generation, "epoch": a.epoch}, ""); err == nil {
		t.Fatal("cancelled download reached player")
	}
	if len(a.journal.Events) != 1 || a.journal.Events[0].State != "stopped" {
		t.Fatal("pending task not completed")
	}
}

func TestOfflineLogsRemainBoundedAndRedacted(t *testing.T) {
	directory := t.TempDir()
	private := "fixture-secret-token"
	if err := os.WriteFile(filepath.Join(directory, "launcher.log"), []byte(strings.Repeat("line\n", 10000)+private+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	text := strings.Join(diagnosticLogs(directory, private, nil), "\n")
	if strings.Contains(text, private) || !strings.Contains(text, "[REDACTED]") || len(text) > 17000 {
		t.Fatal("offline diagnostics leaked credentials or exceeded limit")
	}
}
