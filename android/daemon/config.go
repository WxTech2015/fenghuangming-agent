package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const version = "0.2.0-android.1"
const packageName = "com.phoring.fenghuangming.player"
const defaultData = "/data/adb/fenghuangming"

type Config struct {
	ServerURL   string `json:"serverUrl"`
	Token       string `json:"token"`
	CacheMaxMB  int64  `json:"cacheMaxMb"`
	Connections int    `json:"connections"`
}

func (c *Config) validate() error {
	if c.ServerURL == "" && c.Token == "" {
		return nil
	}
	u, err := url.Parse(c.ServerURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("端点须为 HTTP / HTTPS 根地址")
	}
	c.ServerURL = strings.TrimRight(c.ServerURL, "/")
	if len(c.Token) < 8 || len(c.Token) > 4096 || strings.ContainsAny(c.Token, "\r\n") {
		return errors.New("设备凭证无效")
	}
	if c.CacheMaxMB < 100 || c.CacheMaxMB > 10240 {
		return errors.New("缓存大小须为 100–10240 MB")
	}
	if c.Connections < 1 || c.Connections > 4 {
		return errors.New("下载并发须为 1–4")
	}
	return nil
}
func atomicJSON(file string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	tmp := file + "." + newID() + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, file)
}
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func readJSON(file string, value any) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, value)
}

type PlayerState struct {
	Ready        bool    `json:"ready"`
	PlaybackID   *string `json:"playbackId"`
	Started      bool    `json:"started"`
	Position     float64 `json:"position"`
	Duration     float64 `json:"durationSeconds"`
	Seekable     bool    `json:"seekable"`
	Paused       bool    `json:"paused"`
	Output       string  `json:"output"`
	SystemVolume int     `json:"systemVolume"`
	Error        string  `json:"error,omitempty"`
}
type Payload struct {
	Action      string   `json:"action"`
	PlaybackID  string   `json:"playbackId"`
	QueueItemID string   `json:"queueItemId"`
	AssetID     string   `json:"assetId"`
	Ticket      string   `json:"ticket"`
	SHA256      string   `json:"sha256"`
	Bytes       int64    `json:"bytes"`
	MIME        string   `json:"mime"`
	Volume      int      `json:"volume"`
	Position    float64  `json:"position"`
	Title       string   `json:"title,omitempty"`
	Artists     []string `json:"artists,omitempty"`
}
type Command struct {
	V         int     `json:"v"`
	Kind      string  `json:"kind"`
	ID        string  `json:"id"`
	AgentID   string  `json:"agentId"`
	ZoneID    string  `json:"zoneId"`
	Epoch     int64   `json:"epoch"`
	ExpiresAt string  `json:"expiresAt"`
	Payload   Payload `json:"payload"`
}
type Event struct {
	Type       string `json:"type"`
	EventID    string `json:"eventId"`
	PlaybackID string `json:"playbackId"`
	State      string `json:"state"`
	Error      string `json:"error,omitempty"`
}
type Journal struct {
	Commands []string `json:"commands"`
	Events   []Event  `json:"events"`
}
