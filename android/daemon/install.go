package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type androidRunner func(context.Context, io.Reader, string, ...string) (string, error)

func streamPlayerAPK(module string, run androidRunner) error {
	file, err := os.Open(filepath.Join(module, "assets", "player.apk"))
	if err != nil {
		return errors.New("模块内缺少播放 APK")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 200*1024*1024 {
		return errors.New("播放 APK 文件无效")
	}
	expected, err := os.ReadFile(filepath.Join(module, "assets", "player.sha256"))
	if err != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(strings.TrimSpace(string(expected))) {
		return errors.New("播放 APK 校验记录无效")
	}
	actual, err := hashFile(file.Name())
	if err != nil || actual != strings.TrimSpace(string(expected)) {
		return errors.New("播放 APK 校验失败")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	output, err := run(ctx, file, "/system/bin/pm", "install", "-r", "--user", "0", "-S", strconv.FormatInt(info.Size(), 10), "-")
	if err != nil {
		return fmt.Errorf("播放组件安装失败：%w", err)
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "Success" {
			return nil
		}
	}
	return fmt.Errorf("播放组件安装未确认成功：%s", output)
}
