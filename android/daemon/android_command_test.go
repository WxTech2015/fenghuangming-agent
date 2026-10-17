package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAndroidCommandHelper(t *testing.T) {
	mode := os.Getenv("FHM_TEST_COMMAND")
	if mode == "" {
		return
	}
	if mode == "fail" {
		fmt.Fprintln(os.Stderr, "Failure [INSTALL_FAILED_USER_RESTRICTED]")
		os.Exit(7)
	}
	if mode == "large" {
		fmt.Print(strings.Repeat("x", 1024*1024))
		os.Exit(0)
	}
	if mode == "wait" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	if mode == "pipe" {
		for _, file := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
			info, err := file.Stat()
			if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
				fmt.Fprintln(os.Stderr, "Binder inherited a non-pipe descriptor")
				os.Exit(8)
			}
		}
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(9)
	}
	fmt.Print(string(data))
	fmt.Fprint(os.Stderr, " stderr-ok")
	os.Exit(0)
}

func TestAndroidCommandDiagnostics(t *testing.T) {
	t.Setenv("FHM_TEST_COMMAND", "fail")
	output, err := runAndroidCommand(context.Background(), nil, os.Args[0], "-test.run=^TestAndroidCommandHelper$")
	if err == nil || !strings.Contains(err.Error(), "INSTALL_FAILED_USER_RESTRICTED") || !strings.Contains(output, "INSTALL_FAILED_USER_RESTRICTED") {
		t.Fatalf("lost Android error: %q %v", output, err)
	}
	t.Setenv("FHM_TEST_COMMAND", "large")
	output, err = runAndroidCommand(context.Background(), nil, os.Args[0], "-test.run=^TestAndroidCommandHelper$")
	if err != nil || len(output) != 16384 {
		t.Fatalf("output not bounded/drained: %d %v", len(output), err)
	}
	t.Setenv("FHM_TEST_COMMAND", "wait")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err = runAndroidCommand(ctx, nil, os.Args[0], "-test.run=^TestAndroidCommandHelper$"); err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("missing timeout: %v", err)
	}
}
