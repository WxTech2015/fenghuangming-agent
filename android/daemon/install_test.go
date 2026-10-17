package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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
				if program != "/system/bin/pm" || !reflect.DeepEqual(args, []string{"install", "-r", "--user", "0", "-S", "1600", "-"}) {
					t.Fatalf("unsafe install arguments: %s %v", program, args)
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
