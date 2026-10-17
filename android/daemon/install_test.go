package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 按 Android Shell 的选项规则检查外部接口；单独的 "-" 会被当作未知选项。
// APK 路径省略且提供 -S 时，Android 10 / 14 的包管理器均从标准输入读取。
func checkInstallOptions(args []string, size int) error {
	if len(args) == 0 || args[0] != "install" {
		return errors.New("expected install command")
	}
	var replace bool
	var user, declared string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-r":
			replace = true
		case "--user", "-S":
			option := args[i]
			i++
			if i == len(args) {
				return errors.New("missing option value")
			}
			if option == "--user" {
				user = args[i]
			} else {
				declared = args[i]
			}
		default:
			return fmt.Errorf("Unknown option %s", args[i])
		}
	}
	if !replace || user != "0" || declared != strconv.Itoa(size) {
		return errors.New("missing replacement, user or stream size")
	}
	return nil
}

func TestPlayerInstallStream(t *testing.T) {
	module := t.TempDir()
	assets := filepath.Join(module, "assets")
	if err := os.Mkdir(assets, 0700); err != nil {
		t.Fatal(err)
	}
	apk := bytes.Repeat([]byte("test-apk-payload"), 100)
	sum := sha256.Sum256(apk)
	if err := os.WriteFile(filepath.Join(assets, "player.apk"), apk, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "player.sha256"), []byte(hex.EncodeToString(sum[:])+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, result := range []struct {
		name, output string
		commandError error
		want         string
	}{
		{"success", "Performing Streamed Install\nSuccess\n", nil, ""},
		{"binder failure", "", errors.New("Failed transaction (2147483646)"), "2147483646"},
		{"install rejected", "Failure [INSTALL_FAILED_USER_RESTRICTED]", nil, "INSTALL_FAILED_USER_RESTRICTED"},
	} {
		t.Run(result.name, func(t *testing.T) {
			err := streamPlayerAPK(module, func(ctx context.Context, input io.Reader, program string, args ...string) (string, error) {
				if program != "/system/bin/pm" {
					t.Fatalf("unexpected program: %s", program)
				}
				if err := checkInstallOptions(args, len(apk)); err != nil {
					return "", err
				}
				payload, err := io.ReadAll(input)
				if err != nil || !bytes.Equal(payload, apk) {
					t.Fatal("APK stream changed")
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("missing deadline")
				}
				return result.output, result.commandError
			})
			if result.want == "" && err != nil {
				t.Fatal(err)
			}
			if result.want != "" && (err == nil || !strings.Contains(err.Error(), result.want)) {
				t.Fatalf("missing install detail: %v", err)
			}
		})
	}
	if err := os.WriteFile(filepath.Join(assets, "player.apk"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := streamPlayerAPK(module, func(context.Context, io.Reader, string, ...string) (string, error) {
		t.Fatal("corrupt APK reached package manager")
		return "", nil
	}); err == nil {
		t.Fatal("corrupt APK accepted")
	}
}
