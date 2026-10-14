//go:build windows

package session

import (
	"errors"
	"os"
	"os/exec"
)

// Windows terminals need ConPTY, which is M3 in PLAN.md. Until that lands the
// daemon refuses to start a session instead of reporting success for a shell
// that cannot be typed into.
var errNoConPTY = errors.New("pty: windows (ConPTY) support is not implemented yet")

func startPTY(cmd *exec.Cmd, cols, rows int) (*os.File, error) { return nil, errNoConPTY }

func resizePTY(f *os.File, cols, rows int) error { return errNoConPTY }

func killProcess(pid int) error { return errNoConPTY }

func shellCommand(shell string) (string, []string) {
	if shell != "" {
		return shell, nil
	}
	if c := os.Getenv("COMSPEC"); c != "" {
		return c, nil
	}
	return "cmd.exe", nil
}
