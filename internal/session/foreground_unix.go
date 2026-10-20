//go:build !windows

package session

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Foreground asks the kernel which process group owns the terminal, then asks
// what that program is called: the same answer kero's pane header shows, and
// what lets a fleet say "claude" without the program reporting anything.
func (p *unixPTY) Foreground() (string, int) {
	pgrp, err := unix.IoctlGetInt(int(p.File.Fd()), unix.TIOCGPGRP)
	if err != nil || pgrp <= 0 {
		return "", 0
	}
	return filepath.Base(processName(pgrp)), pgrp
}
