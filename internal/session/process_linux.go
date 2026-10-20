//go:build linux

package session

import (
	"os"
	"strconv"
	"strings"
)

// processName is a process's name, from procfs: no fork, no permissions to ask
// for beyond reading the process's own comm file.
func processName(pid int) string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
