package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

type commandLog struct{ text []byte }

func (b *commandLog) Write(p []byte) (int, error) {
	if remaining := 16384 - len(b.text); remaining > 0 {
		b.text = append(b.text, p[:min(len(p), remaining)]...)
	}
	return len(p), nil
}

// Binder 会接收命令的标准流；不要把 Root 日志、APK 文件或管理器终端直接交给系统服务。
func runAndroidCommand(ctx context.Context, input io.Reader, program string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, program, args...)
	command.WaitDelay = 2 * time.Second
	if input != nil {
		command.Stdin = io.LimitReader(input, 200*1024*1024+1)
	}
	var output commandLog
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	text := strings.TrimSpace(string(output.text))
	if ctx.Err() != nil {
		return text, fmt.Errorf("Android 命令超时或取消：%w", ctx.Err())
	}
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, fmt.Errorf("Android 命令失败：%s", text)
	}
	return text, nil
}

func commandOutput(program string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return runAndroidCommand(ctx, nil, program, args...)
}
