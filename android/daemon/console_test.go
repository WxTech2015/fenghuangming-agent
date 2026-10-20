package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsoleRestrictsOriginAuthAndCommands(t *testing.T) {
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "webroot"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "webroot", "index.html"), []byte("console"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "config.json"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("a", 64)
	calls := 0
	handler := consoleHandler(directory, key, func(_ context.Context, service bool, payload []byte) ([]byte, error) {
		calls++
		return []byte(`{"ok":true}`), nil
	})
	for _, test := range []struct {
		path, host, origin, auth, body string
		status                         int
	}{
		{"/", consoleAddress, "", "", "", 200},
		{"/config.json", consoleAddress, "", "", "", 404},
		{"/assets/../../config.json", consoleAddress, "", "", "", 404},
		{"/api/request", "attacker.example", "", key, `{"action":"state"}`, 403},
		{"/api/request", consoleAddress, "https://attacker.example", key, `{"action":"state"}`, 403},
		{"/api/request", consoleAddress, consoleOrigin, "wrong", `{"action":"state"}`, 401},
		{"/api/service", consoleAddress, consoleOrigin, key, `{"action":"shell","command":"id"}`, 400},
		{"/api/request", consoleAddress, consoleOrigin, key, `{"action":"state"}`, 200},
		{"/api/service", consoleAddress, consoleOrigin, key, `{"action":"stop"}`, 200},
	} {
		method := http.MethodGet
		if test.body != "" {
			method = http.MethodPost
		}
		r := httptest.NewRequest(method, consoleOrigin+test.path, strings.NewReader(test.body))
		r.Host = test.host
		r.Header.Set("Origin", test.origin)
		r.Header.Set("Authorization", "Bearer "+test.auth)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Header().Get("Referrer-Policy") != "same-origin" {
			t.Fatal("old WebView origin policy changed")
		}
		if w.Code != test.status {
			t.Errorf("%s: got %d, want %d: %s", test.path, w.Code, test.status, w.Body.String())
		}
	}
	if calls != 2 {
		t.Fatalf("unexpected commands: %d", calls)
	}
}

func TestConsoleKeyIsPersistentPrivateAndNotReturnedByState(t *testing.T) {
	directory := t.TempDir()
	key, err := consoleKey(directory)
	if err != nil {
		t.Fatal(err)
	}
	next, err := consoleKey(directory)
	if err != nil || key != next || len(key) != 64 {
		t.Fatal("key was not persistent", err)
	}
	a, err := newAgent(directory, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := json.Marshal(a.state())
	if err != nil || strings.Contains(string(state), key) {
		t.Fatal("key leaked in state")
	}
}
