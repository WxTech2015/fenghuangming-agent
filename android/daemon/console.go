package main

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const consoleAddress = "127.0.0.1:17789"
const consoleOrigin = "http://" + consoleAddress

func consoleKey(data string) (string, error) {
	file := filepath.Join(data, "console-key.json")
	if info, err := os.Lstat(file); err == nil {
		if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			return "", errors.New("控制台密钥文件权限不正确")
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	var value struct {
		Key string `json:"key"`
	}
	if err := readJSON(file, &value); os.IsNotExist(err) {
		value.Key = newID() + newID()
		if err = atomicJSON(file, value); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	if !hashPattern.MatchString(value.Key) {
		return "", errors.New("控制台密钥文件损坏")
	}
	return value.Key, nil
}

type consoleRunner func(context.Context, bool, []byte) ([]byte, error)

func consoleCommand(data, directory string) consoleRunner {
	return func(ctx context.Context, service bool, payload []byte) ([]byte, error) {
		var command *exec.Cmd
		if service {
			var value struct {
				Action string `json:"action"`
			}
			if err := json.Unmarshal(payload, &value); err != nil {
				return nil, err
			}
			switch value.Action {
			case "start", "stop", "install":
			default:
				return nil, errors.New("不支持的服务操作")
			}
			command = exec.CommandContext(ctx, "/system/bin/sh", filepath.Join(directory, "scripts", "manage.sh"), value.Action)
		} else {
			command = exec.CommandContext(ctx, filepath.Join(directory, "bin", "fhm-agent"), "--data", data, "--request", base64.StdEncoding.EncodeToString(payload))
		}
		output, err := command.Output()
		if err != nil {
			return nil, errors.New("本机命令失败，请查看运行日志")
		}
		if service {
			return []byte(`{"ok":true}`), nil
		}
		if !json.Valid(output) {
			return nil, errors.New("本机响应无效")
		}
		return output, nil
	}
}

func consoleHandler(directory, key string, run consoleRunner) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if r.Host != consoleAddress || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != consoleOrigin) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `{"ok":false,"error":"请求来源无效"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+key)) != 1 {
				w.WriteHeader(http.StatusUnauthorized)
				io.WriteString(w, `{"ok":false,"error":"请通过脚本打开控制台登录地址"}`)
				return
			}
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
				http.Error(w, `{"ok":false,"error":"请求格式无效"}`, http.StatusBadRequest)
				return
			}
			if r.URL.Path != "/api/request" && r.URL.Path != "/api/service" {
				http.NotFound(w, r)
				return
			}
			payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32768))
			var value struct {
				Action string `json:"action"`
			}
			if err != nil || json.Unmarshal(payload, &value) != nil {
				http.Error(w, `{"ok":false,"error":"管理请求无效或过大"}`, http.StatusBadRequest)
				return
			}
			allowed := map[string]bool{"state": true, "logs": true, "configure": true, "brake": true, "release": true, "activate": true, "pause": true, "resume": true, "stop": true, "seek": true, "volume": true}
			service := r.URL.Path == "/api/service"
			if service {
				allowed = map[string]bool{"start": true, "stop": true, "install": true}
			}
			if !allowed[value.Action] {
				http.Error(w, `{"ok":false,"error":"不支持的管理操作"}`, http.StatusBadRequest)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			output, err := run(ctx, service, payload)
			if err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
				return
			}
			w.Write(output)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		name := r.URL.Path
		if name == "/" {
			name = "/index.html"
		}
		if name != "/index.html" && !strings.HasPrefix(name, "/assets/") {
			http.NotFound(w, r)
			return
		}
		root := filepath.Join(directory, "webroot")
		file := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(name, "/")))
		relative, err := filepath.Rel(root, file)
		if err != nil || strings.HasPrefix(relative, "..") {
			http.NotFound(w, r)
			return
		}
		info, err := os.Lstat(file)
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, file)
	})
}

func serveConsole(ctx context.Context, data, directory, key string) error {
	listener, err := net.Listen("tcp4", consoleAddress)
	if err != nil {
		return fmt.Errorf("本机控制台端口不可用：%w", err)
	}
	defer listener.Close()
	server := &http.Server{Handler: consoleHandler(directory, key, consoleCommand(data, directory)), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	go func() { <-ctx.Done(); _ = server.Close() }()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
