//go:build !windows

package session

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

func startPTY(cmd *exec.Cmd, cols, rows int) (*os.File, error) {
	return pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func resizePTY(f *os.File, cols, rows int) error {
	return pty.Setsize(f, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

// killProcess ends the whole process group, so a shell's children go with it
// instead of being reparented and left running.
func killProcess(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		return syscall.Kill(pid, syscall.SIGKILL)
	}
	return nil
}

func shellCommand(shell string) (string, []string) {
	if shell != "" {
		return shell, nil
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh, nil
	}
	return "/bin/sh", nil
}
