//go:build darwin

package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

// launchd runs the daemon as a per-user agent: it starts at login and is
// restarted when it exits. The agent lives in the user's own LaunchAgents
// directory, so it needs no root and its shells run as the person who
// installed it.
func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"), nil
}

// Install writes the agent and loads it.
func Install(c Config) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	path, err := plistPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if c.LogPath == "" {
		home, _ := os.UserHomeDir()
		c.LogPath = filepath.Join(home, "Library", "Logs", "Beacon", "member.log")
	}
	if err := os.MkdirAll(filepath.Dir(c.LogPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(plistText(c)), 0o600); err != nil {
		return "", err
	}
	// A previous load of the same label has to go first, or bootstrap refuses
	// the file it already knows.
	_ = run("launchctl", "bootout", domain()+"/"+Label)
	if err := run("launchctl", "bootstrap", domain(), path); err != nil {
		return "", err
	}
	_ = run("launchctl", "enable", domain()+"/"+Label)
	return "installed " + path, nil
}

// Uninstall unloads the agent and removes it.
func Uninstall() error {
	_ = run("launchctl", "bootout", domain()+"/"+Label)
	path, err := plistPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Restart kills and relaunches the daemon. Its sessions end with it.
func Restart() error { return run("launchctl", "kickstart", "-k", domain()+"/"+Label) }

// Status reports what launchd says.
func Status() (string, error) {
	out, err := exec.Command("launchctl", "print", domain()+"/"+Label).CombinedOutput()
	if err != nil {
		return "not loaded", nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "state = ") {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "state = ")), nil
		}
	}
	return "loaded", nil
}

// domain is the launchd session the agent belongs to.
func domain() string {
	if u, err := user.Current(); err == nil && u.Uid != "" {
		return "gui/" + u.Uid
	}
	return "gui/" + fmt.Sprint(os.Getuid())
}

// plistText is the agent as launchd reads it.
func plistText(c Config) string {
	var args strings.Builder
	for _, a := range c.ExecArgs() {
		fmt.Fprintf(&args, "\t\t<string>%s</string>\n", xmlEscape(a))
	}
	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
%s	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>%s</string>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>3</integer>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, Label, args.String(), xmlEscape(path), xmlEscape(c.LogPath), xmlEscape(c.LogPath))
}

// ExecArgs is the binary and its flags, unquoted: plist arguments are already
// one string each.
func (c Config) ExecArgs() []string { return append([]string{c.Bin}, c.Args...) }

// xmlEscape makes a value safe inside a plist string.
func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
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
