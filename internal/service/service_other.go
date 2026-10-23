//go:build !linux && !darwin && !windows

package service

import "errors"

// Elsewhere there is no service manager this package knows how to write to. The
// daemon still runs; it just has to be started by whatever the system uses.
func Install(Config) (string, error) {
	return "", errors.New("service: this platform has no supported service manager")
}

func Uninstall() error { return errors.New("service: this platform has no supported service manager") }

func Restart() error { return errors.New("service: this platform has no supported service manager") }

func Status() (string, error) { return "unsupported", nil }
