package session

import "io"

// ptyProcess is a child process attached to a terminal multiplexer: one byte
// stream, one window size, one process to reap.
//
// The interface exists because the two hosts are genuinely different animals.
// On Unix a PTY is a file descriptor you read and write, and a child is an
// *exec.Cmd. On Windows there is no such thing: ConPTY is a pseudo console
// object that owns the process it was created with, so waiting on the process
// is a call on the console, not on an exec.Cmd. Everything above this line —
// the ring buffer, the cursor arithmetic, the control arbitration — is
// identical on both.
type ptyProcess interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
	// Wait blocks until the process exits and reports its exit code.
	Wait() (int, error)
	PID() int
}

// ptySpec is what to launch, described the same way on both hosts.
type ptySpec struct {
	path string
	args []string
	env  []string
	dir  string
}
