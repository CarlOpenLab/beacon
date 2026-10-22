//go:build !windows

package session

import "testing"

// The macOS preference names a locale the way `locale -a` does, and a language
// tag, which the C library does not know, names nothing rather than a locale
// that would make every shell complain at startup.
func TestLocaleName(t *testing.T) {
	tests := map[string]string{
		"zh_CN":                "zh_CN.UTF-8",
		"en_US\n":              "en_US.UTF-8",
		"en_US@currency=USD":   "en_US.UTF-8",
		"zh-Hans-CN":           "",
		"":                     "",
		"en_US.UTF-8":          "",
		"somewhere/else.UTF-8": "",
	}
	for in, want := range tests {
		if got := localeName(in); got != want {
			t.Errorf("localeName(%q) = %q, want %q", in, got, want)
		}
	}
}
