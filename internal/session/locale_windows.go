//go:build windows

package session

// utf8Locale is "" on Windows: a ConPTY hands its shell UTF-16 code units and
// there is no locale for the environment to name.
func utf8Locale() string { return "" }
