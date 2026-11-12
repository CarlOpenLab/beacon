//go:build !windows

package session

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

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

// Resize asks the kernel to change the PTY's window.
//
// The descriptor is taken through the file's SyscallConn rather than its Fd().
// Fd() reads the runtime's descriptor field with no reference held, so the last
// read to return — pump's, when the shell's output ends — can clear that field
// while this ioctl is reading it, which is a race between two goroutines that
// never touch each other's data. Control() holds a reference to the descriptor
// for the length of the call, so the field cannot be cleared underneath it. The
// library's Setsize does not, so the ioctl is issued here instead.
func (p *unixPTY) Resize(cols, rows int) error {
	ws := pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}
	conn, err := p.File.SyscallConn()
	if err != nil {
		return err
	}
	var ioctlErr error
	if err := conn.Control(func(fd uintptr) {
		ioctlErr = ioctlWinsize(fd, &ws)
	}); err != nil {
		return err
	}
	return ioctlErr
}

// ioctlWinsize sets the window size on a descriptor held by the caller.
func ioctlWinsize(fd uintptr, ws *pty.Winsize) error {
	//nolint:gosec // Expected unsafe pointer for Syscall call.
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(ws)))
	if errno != 0 {
		return errno
	}
	return nil
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
