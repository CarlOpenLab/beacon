//go:build darwin

package main

import "testing"

// The window corner is decided from the OS version at startup; if the read
// fails, the card falls back to the shape of a modern macOS window, which is a
// visible mismatch on an older one. The read itself is what this checks.
func TestMacMajorVersionReads(t *testing.T) {
	if v := macMajorVersion(); v < 13 {
		t.Fatalf("macOS major version = %d; the kernel read failed", v)
	} else {
		t.Logf("macOS major version %d", v)
	}
}
