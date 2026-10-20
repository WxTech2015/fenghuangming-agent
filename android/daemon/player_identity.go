package main

import (
	"strconv"
	"strings"
)

func isPlayerProcess(status, cmdline string, uid uint32) bool {
	if uid < 10000 || strings.SplitN(cmdline, "\x00", 2)[0] != packageName {
		return false
	}
	for _, line := range strings.Split(status, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 5 && fields[0] == "Uid:" {
			for _, value := range fields[1:] {
				actual, err := strconv.ParseUint(value, 10, 32)
				if err != nil || uint32(actual) != uid {
					return false
				}
			}
			return true
		}
	}
	return false
}
