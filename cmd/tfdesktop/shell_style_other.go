//go:build !darwin

package main

// macMajorVersion is 0 away from macOS: there is no macOS window corner to
// follow, so the caller keeps the value its own platform names.
func macMajorVersion() int { return 0 }
