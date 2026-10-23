// Package service installs the member daemon as the operating system's own
// service: a systemd user unit on Linux, a launchd agent on macOS, a logon task
// on Windows.
//
// The reason is that a terminal hub is only useful if it is already there. A
// `tfd` started from a shell dies with the shell, and a machine that has to be
// re-armed after every reboot is a machine that is offline when the phone
// reaches for it. The credential stays in the unit the installer writes, in the
// user's own directory, which is also why this is a user service: every shell
// the daemon starts belongs to that user, with that user's files and PATH.
package service

import (
	"fmt"
	"strings"
)

// Name is the service's name: the systemd unit, the launchd label's last part,
// and the Windows task.
const (
	Name  = "beacon-member"
	Label = "dev.carl.beacon.member"
)

// Config is what a unit runs: the daemon's binary, the flags it was asked to
// run with, and where a service manager that does not capture output writes it.
type Config struct {
	// Bin is the absolute path to the tfd executable.
	Bin string
	// Args are the flags after it, exactly as the person typed them.
	Args []string
	// LogPath is where stdout and stderr go on managers that want a file
	// (launchd). systemd keeps them in the journal.
	LogPath string
}

// Exec is the command line a service manager runs: the path and each argument
// quoted for a POSIX shell. A path with a space in it is the normal case on
// macOS, and a credential with a quote in it must not become two arguments.
func (c Config) Exec() string {
	parts := append([]string{c.Bin}, c.Args...)
	for i, p := range parts {
		parts[i] = shellQuote(p)
	}
	return strings.Join(parts, " ")
}

// shellQuote quotes one argument for a POSIX shell. systemd splits ExecStart on
// whitespace unless an argument is quoted, and both it and launchd read the
// same convention.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Validate rejects a config that would install a service that cannot start.
func (c Config) Validate() error {
	if c.Bin == "" {
		return fmt.Errorf("service: no binary path")
	}
	if !strings.HasPrefix(c.Bin, "/") && !strings.Contains(c.Bin, `:\`) {
		return fmt.Errorf("service: %q is not an absolute path", c.Bin)
	}
	return nil
}
