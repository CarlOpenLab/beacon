//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Windows has no user service; the nearest thing a plain user can create is a
// logon task, which is what the installer uses. The command line goes in a small
// .cmd file rather than in the task itself: nesting quotes through schtasks
// /TR is a way to get a task that looks right in the UI and runs something
// else, and a file with one line has no such ambiguity.
const taskName = "Beacon member"

func scriptPath() (string, error) {
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(dir, "Beacon", "member.cmd"), nil
}

// Install writes the task and starts it.
func Install(c Config) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	path, err := scriptPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	script := "@echo off\r\n" + cmdLine(c) + "\r\n"
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		return "", err
	}
	if err := run("schtasks", "/Create", "/TN", taskName, "/TR", path, "/SC", "ONLOGON", "/RL", "LIMITED", "/F"); err != nil {
		return "", err
	}
	_ = run("schtasks", "/Run", "/TN", taskName)
	return "installed " + path, nil
}

// Uninstall removes the task and its script.
func Uninstall() error {
	_ = run("schtasks", "/End", "/TN", taskName)
	_ = run("schtasks", "/Delete", "/TN", taskName, "/F")
	path, err := scriptPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Restart ends the task and runs it again. Its sessions end with it.
func Restart() error {
	_ = run("schtasks", "/End", "/TN", taskName)
	return run("schtasks", "/Run", "/TN", taskName)
}

// Status reports whether the logon task exists.
func Status() (string, error) {
	out, err := exec.Command("schtasks", "/Query", "/TN", taskName).CombinedOutput()
	if err != nil {
		return "not installed", nil
	}
	if strings.Contains(string(out), "Running") {
		return "running", nil
	}
	return "installed", nil
}

// cmdLine is one command line for cmd.exe. An argument with a space or a quote
// is wrapped, and a quote inside is doubled the way cmd expects at the end of a
// quoted token.
func cmdLine(c Config) string {
	parts := append([]string{c.Bin}, c.Args...)
	for i, p := range parts {
		if strings.ContainsAny(p, " \t\"") {
			parts[i] = `"` + strings.ReplaceAll(p, `"`, `""`) + `"`
		}
	}
	return strings.Join(parts, " ")
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return fmt.Errorf("%s: %s", name, msg)
	}
	return nil
}
