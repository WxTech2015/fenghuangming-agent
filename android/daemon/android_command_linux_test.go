//go:build linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBinderReceivesPipes(t *testing.T) {
	t.Setenv("FHM_TEST_COMMAND", "pipe")
	path := filepath.Join(t.TempDir(), "root-private.apk")
	if err := os.WriteFile(path, []byte("apk-stream"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	output, err := runAndroidCommand(context.Background(), file, os.Args[0], "-test.run=^TestAndroidCommandHelper$")
	if err != nil || output != "apk-stream stderr-ok" {
		t.Fatalf("standard streams were not isolated: %q %v", output, err)
	}
}
