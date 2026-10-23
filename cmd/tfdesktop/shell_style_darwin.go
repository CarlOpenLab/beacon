//go:build darwin

package main

import (
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

var (
	macMajorOnce sync.Once
	macMajor     int
)

// macMajorVersion is the macOS version's major number, read from the kernel.
// It decides which window corner the system is drawing: 26 (Tahoe) made windows
// much rounder than the 10 that every release before it used.
func macMajorVersion() int {
	macMajorOnce.Do(func() {
		v, err := unix.Sysctl("kern.osproductversion")
		if err != nil {
			return
		}
		major, _, _ := strings.Cut(strings.TrimSpace(v), ".")
		macMajor, _ = strconv.Atoi(major)
	})
	return macMajor
}
