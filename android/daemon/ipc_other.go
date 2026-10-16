//go:build !linux

package main

import (
	"errors"
	"net"
)

func connectPlayer(string) (playerConnection, error) {
	return nil, errors.New("播放器 IPC 仅用于 Android / Linux")
}
func listenControl(string) (net.Listener, error) {
	return nil, errors.New("控制 IPC 仅用于 Android / Linux")
}
func controlPeer(net.Conn) bool         { return false }
func checkSpace(string, int64) error    { return nil }
func activatePlayer() error             { return errors.New("仅用于 Android") }
func stopPlayer() error                 { return errors.New("仅用于 Android") }
func lockDaemon(string) (func(), error) { return func() {}, nil }
