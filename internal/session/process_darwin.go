//go:build darwin

package session

import "golang.org/x/sys/unix"

// processName is a process's name, from the kernel's own process table:
// kern.proc.pid answers with the command name the way ps reads it, without a
// fork per poll.
func processName(pid int) string {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return ""
	}
	name := make([]byte, 0, len(kp.Proc.P_comm))
	for _, c := range kp.Proc.P_comm {
		if c == 0 {
			break
		}
		name = append(name, byte(c))
	}
	return string(name)
}
