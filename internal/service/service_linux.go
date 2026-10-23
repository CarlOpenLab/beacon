//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// systemd runs the daemon as a user unit: it starts at login, restarts after a
// crash, and with linger enabled it keeps running when the person logs out.
// That last part is the difference between a machine that is reachable and one
// that is only reachable while someone is sitting at it.
func unitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", Name+".service"), nil
}

// Install writes the unit and starts it.
func Install(c Config) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	path, err := unitPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	unit := unitText(c)
	if err := os.WriteFile(path, []byte(unit), 0o600); err != nil {
		return "", err
	}
	if err := run("systemctl", "--user", "daemon-reload"); err != nil {
		return "", err
	}
	if err := run("systemctl", "--user", "enable", Name+".service"); err != nil {
		return "", err
	}
	// restart, not start: on a second run the unit is already active with the
	// binary this install just replaced, and an "already running" answer would
	// leave the old process serving the new file's name.
	if err := run("systemctl", "--user", "restart", Name+".service"); err != nil {
		return "", err
	}
	// Keep the service alive after logout. It is best effort: a machine
	// without logind still has a working unit while the user is logged in.
	_ = run("loginctl", "enable-linger", os.Getenv("USER"))
	return "installed " + path, nil
}

// Uninstall stops and removes the unit.
func Uninstall() error {
	_ = run("systemctl", "--user", "disable", "--now", Name+".service")
	path, err := unitPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return run("systemctl", "--user", "daemon-reload")
}

// Restart asks systemd to restart the running daemon. Sessions end with it:
// the PTYs are its children, and a restart is the one operation that cannot
// pretend otherwise.
func Restart() error { return run("systemctl", "--user", "restart", Name+".service") }

// Status reports what systemd says.
func Status() (string, error) {
	out, err := exec.Command("systemctl", "--user", "is-active", Name+".service").Output()
	state := strings.TrimSpace(string(out))
	if state == "" {
		state = "not installed"
	}
	return state, err
}

// unitText is the unit as systemd reads it.
func unitText(c Config) string {
	return fmt.Sprintf(`[Unit]
Description=Beacon member (terminal hub)
Documentation=https://github.com/CarlOpenLab/beacon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s
Restart=always
RestartSec=3
NoNewPrivileges=true
ProtectSystem=full
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictRealtime=true
LockPersonality=true
# ProtectHome is deliberately not set: reaching your files is the point.

[Install]
WantedBy=default.target
`, c.Exec())
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
