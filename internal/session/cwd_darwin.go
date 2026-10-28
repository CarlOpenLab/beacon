//go:build darwin

package session

import (
	"syscall"
	"unsafe"
)

const (
	sysProcInfo          = 336
	procInfoCallPidInfo  = 2
	procPidVnodePathInfo = 9
)

// darwinLiveCwd reads a process's current working directory directly from the
// kernel via __proc_info(PROC_INFO_CALL_PIDINFO, pid, PROC_PIDVNODEPATHINFO).
// This takes ~2 microseconds and avoids forking /usr/sbin/lsof on every check.
func darwinLiveCwd(pid int) string {
	if pid <= 0 {
		return ""
	}
	var buf [2352]byte
	r1, _, err := syscall.Syscall6(
		sysProcInfo,
		uintptr(procInfoCallPidInfo),
		uintptr(pid),
		uintptr(procPidVnodePathInfo),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if err != 0 || r1 == 0 {
		return ""
	}
	// struct proc_vnodepathinfo has pvi_cdir.vip_path starting at byte offset 152.
	pathBytes := buf[152:]
	for i, b := range pathBytes {
		if b == 0 {
			return string(pathBytes[:i])
		}
	}
	return string(pathBytes)
}
