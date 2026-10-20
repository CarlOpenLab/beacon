//go:build windows

package session

// liveCwd is where a process stands on Windows. A ConPTY owns the process it
// launched, and asking Windows for another process's working directory is a
// handle-and-query dance with no cheap answer, so the caller's fallback — the
// directory the session was started in — is what Windows gets.
func liveCwd(pid int) string { return "" }
