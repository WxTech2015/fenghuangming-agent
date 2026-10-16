package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// 只读取本项目的固定日志文件，限制读取量并隐藏设备凭证。
func diagnosticLogs(data, token string, recent []string) []string {
	lines := append([]string(nil), recent...)
	if len(lines) == 0 {
		file := filepath.Join(data, "logs.json")
		if info, err := os.Stat(file); err == nil && info.Size() <= 131072 {
			_ = readJSON(file, &lines)
		}
	}
	for _, name := range []string{"launcher.log", "daemon.log"} {
		file, err := os.Open(filepath.Join(data, name))
		if err != nil {
			continue
		}
		info, err := file.Stat()
		if err == nil && info.Mode().IsRegular() {
			start := max(int64(0), info.Size()-16384)
			_, _ = file.Seek(start, io.SeekStart)
			bytes, _ := io.ReadAll(io.LimitReader(file, 16384))
			text := string(bytes)
			if start > 0 {
				if at := strings.IndexByte(text, '\n'); at >= 0 {
					text = text[at+1:]
				}
			}
			if strings.TrimSpace(text) != "" {
				lines = append(lines, "["+name+"]", strings.TrimSpace(text))
			}
		}
		_ = file.Close()
	}
	if len(lines) > 240 {
		lines = lines[len(lines)-240:]
	}
	for i := range lines {
		if token != "" {
			lines[i] = strings.ReplaceAll(lines[i], token, "[REDACTED]")
		}
	}
	return lines
}
