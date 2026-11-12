//go:build !windows

package session

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

// Foreground asks the kernel which process group owns the terminal, then asks
// what that program is called: the same answer the pane header shows, and
// what lets a fleet say "claude" without the program reporting anything.
//
// The descriptor is taken through the file's SyscallConn rather than its Fd(),
// for the reason unixPTY.Resize gives: Fd() reads the runtime's descriptor field
// with no reference held, and the last read to return clears that field while
// this ioctl is reading it. Control() holds a reference for the length of the
// call, so the field cannot be cleared underneath it.
func (p *unixPTY) Foreground() (string, int) {
	conn, err := p.File.SyscallConn()
	if err != nil {
		return "", 0
	}
	var pgrp int
	var ioctlErr error
	if err := conn.Control(func(fd uintptr) {
		pgrp, ioctlErr = ioctlPgrp(fd)
	}); err != nil || ioctlErr != nil {
		return "", 0
	}
	if pgrp <= 0 {
		return "", 0
	}
	return filepath.Base(processName(pgrp)), pgrp
}

// ioctlPgrp reads the foreground process group off a descriptor held by the
// caller.
func ioctlPgrp(fd uintptr) (int, error) {
	var pgrp int
	//nolint:gosec // Expected unsafe pointer for Syscall call.
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGPGRP,
		uintptr(unsafe.Pointer(&pgrp)))
	if errno != 0 {
		return 0, errno
	}
	return pgrp, nil
}
