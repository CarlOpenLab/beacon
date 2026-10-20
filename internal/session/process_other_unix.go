//go:build !windows && !linux && !darwin

package session

// processName cannot be read on this host, so a session reports no foreground
// program rather than guessing at one.
func processName(int) string { return "" }
