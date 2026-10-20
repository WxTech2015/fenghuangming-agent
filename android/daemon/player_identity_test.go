package main

import "testing"

func TestPlayerProcessIdentity(t *testing.T) {
	status := "Name:\tfhm\nUid:\t10123\t10123\t10123\t10123\n"
	for _, value := range []struct {
		name, status, cmd string
		uid               uint32
		valid             bool
	}{
		{"player", status, packageName + "\x00", 10123, true},
		{"other uid", status, packageName, 10124, false},
		{"root", "Uid: 0 0 0 0", packageName, 0, false},
		{"other app", status, packageName + ".other\x00", 10123, false},
		{"argument spoof", status, "sh\x00" + packageName, 10123, false},
		{"changed effective uid", "Uid: 10123 0 10123 10123", packageName, 10123, false},
		{"missing uid", "", packageName, 10123, false},
	} {
		t.Run(value.name, func(t *testing.T) {
			if actual := isPlayerProcess(value.status, value.cmd, value.uid); actual != value.valid {
				t.Fatalf("identity=%v", actual)
			}
		})
	}
}
