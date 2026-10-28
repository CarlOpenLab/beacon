//go:build !darwin && !windows

package session

func darwinLiveCwd(pid int) string {
	return ""
}
