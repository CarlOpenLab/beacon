package session

import (
	"os"
	"strings"
)

// shellEnv is the environment a session's shell starts with: this process's
// own, the terminal name a program reads, and — where a system names locales —
// a UTF-8 locale when the environment names no locale at all.
//
// The desktop app is started by the window server, and its environment has no
// LANG. A shell that inherits that comes up in the C locale, where a Chinese
// name is not a name but a run of bytes: zsh echoes 中文 as reverse-video
// <00ad><0096><0087> as it is typed, and answers `cd 中文` with "no such file or
// directory" for a directory that is right there. Terminal.app names a UTF-8
// locale for the shells it starts, which is why the same keystrokes work there;
// this names one too.
//
// A locale the environment already names is left alone, including an explicit
// C: someone who ran `LANG=C tfd` asked for it.
func shellEnv() []string {
	env := os.Environ()
	// A pty is an xterm-256color whatever this process was started from.
	env = withEnv(env, "TERM", "xterm-256color")
	if namesLocale(env) {
		return env
	}
	if loc := utf8Locale(); loc != "" {
		env = withEnv(env, "LANG", loc)
	}
	return env
}

// withEnv returns env with key set to value, replacing what it had: a process's
// environment may name a variable more than once, and which of the copies a C
// library reads is its own business.
func withEnv(env []string, key, value string) []string {
	out := env[:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return append(out, key+"="+value)
}

// namesLocale reports whether env names a locale a shell would run in. An empty
// value names nothing: a unit that cleared the variable says no more than a
// process that never had one.
func namesLocale(env []string) bool {
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		for _, kv := range env {
			if v, ok := strings.CutPrefix(kv, key+"="); ok && v != "" {
				return true
			}
		}
	}
	return false
}

// pickUTF8Locale chooses the locale to name out of the ones the system has,
// the user's own first so that a zh_CN Mac reads zh_CN.UTF-8 as Terminal.app
// does, then the names every system builds with. It answers the spelling the
// system listed, because that is the one it will accept: glibc knows
// "en_US.utf8" where macOS knows "en_US.UTF-8".
//
// It answers "" when the system has none of them. Naming a locale a system does
// not have would print "setlocale: LC_CTYPE: cannot change locale" at the top
// of every shell, which is worse than a shell in the C locale.
func pickUTF8Locale(preferred string, available []string) string {
	for _, want := range []string{preferred, "en_US.UTF-8", "C.UTF-8"} {
		if want == "" {
			continue
		}
		for _, has := range available {
			if sameLocale(has, want) {
				return has
			}
		}
	}
	return ""
}

// sameLocale reports whether two names are one locale written two ways, which
// the systems do: "en_US.UTF-8", "en_US.utf8" and "C.utf8" against "C.UTF-8".
func sameLocale(a, b string) bool {
	norm := func(name string) string {
		return strings.ReplaceAll(strings.ToLower(name), "-", "")
	}
	return norm(a) == norm(b)
}
