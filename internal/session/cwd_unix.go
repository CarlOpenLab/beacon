//go:build !windows

package session

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// liveCwd asks the operating system where a process stands. Linux keeps it in
// /proc; macOS queries the kernel directly via proc_pidinfo. Anything else,
// and any failure, falls back to lsof, and finally to the started directory.
func liveCwd(pid int) string {
	if pid <= 0 {
		return ""
	}
	if _, err := os.Stat("/proc"); err == nil {
		if dir, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd"); err == nil && dir != "" {
			return dir
		}
		return ""
	}
	if dir := darwinLiveCwd(pid); dir != "" {
		return dir
	}
	return lsofCwd(pid)
}

// lsofCwd reads the process's working directory out of lsof. The answer is one
// line, but lsof is another process that can hang on a busy machine, so it is
// given a short deadline rather than being allowed to hold a shell's birth.
func lsofCwd(pid int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/sbin/lsof",
		"-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil {
		return ""
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "n") {
			if dir := line[1:]; dir != "" {
				return dir
			}
		}
	}
	return ""
}
