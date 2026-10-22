//go:build !windows

package session

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// utf8Locale is the UTF-8 locale a shell is given when its environment names
// no locale: the user's own where the system has it, en_US.UTF-8 otherwise.
// It is answered once — it takes two programs to answer, and what the system
// offers does not change while the app runs.
var utf8LocaleOnce struct {
	once sync.Once
	name string
}

func utf8Locale() string {
	utf8LocaleOnce.once.Do(func() {
		utf8LocaleOnce.name = pickUTF8Locale(preferredLocale(), systemLocales())
	})
	return utf8LocaleOnce.name
}

// preferredLocale is the locale the user chose in System Settings, in the form
// a shell's environment wants it ("zh_CN.UTF-8"), or "" where the system keeps
// that somewhere other than the macOS preference this reads.
func preferredLocale() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	out, err := probe("/usr/bin/defaults", "read", "-g", "AppleLocale")
	if err != nil {
		return ""
	}
	return localeName(string(out))
}

// localeName turns the macOS preference, "zh_CN" or "en_US@currency=USD", into
// the UTF-8 name a shell would be given. A tag such as AppleLanguages' shape,
// "zh-Hans-CN", is not a locale the C library knows, and comes back "".
func localeName(apple string) string {
	name := strings.TrimSpace(apple)
	if at, _, found := strings.Cut(name, "@"); found {
		name = at
	}
	if name == "" {
		return ""
	}
	for _, r := range name {
		switch {
		case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', '0' <= r && r <= '9', r == '_':
		default:
			return ""
		}
	}
	return name + ".UTF-8"
}

// systemLocales lists the locales the system's C library has, from `locale -a`.
// nil when the command is missing or does not answer: the caller then names no
// locale rather than guess at one.
func systemLocales() []string {
	out, err := probe("locale", "-a")
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// probe runs a program for its output, under a deadline: a shell waiting to
// start must not wait long on a lookup that hung.
func probe(path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, path, args...).Output()
}
