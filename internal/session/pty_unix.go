//go:build !windows

package session

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

// unixPTY pairs the PTY file with the process attached to it.
type unixPTY struct {
	*os.File
	cmd *exec.Cmd
}

func startPTY(spec ptySpec, cols, rows int) (ptyProcess, error) {
	cmd := exec.Command(spec.path, spec.args...)
	cmd.Env = spec.env
	if spec.dir != "" {
		cmd.Dir = spec.dir
	}
	// StartWithSize puts the child in a new session with the PTY as its
	// controlling terminal, so a kill on the process group takes the shell's
	// children with it.
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	return &unixPTY{File: f, cmd: cmd}, nil
}

func (p *unixPTY) Resize(cols, rows int) error {
	return pty.Setsize(p.File, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (p *unixPTY) Wait() (int, error) {
	err := p.cmd.Wait()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return -1, err
}

func (p *unixPTY) PID() int { return p.cmd.Process.Pid }

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
