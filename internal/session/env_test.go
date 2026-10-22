package session

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// A shell started from the window server has no LANG to inherit, and a shell in
// the C locale reads a Chinese name one byte at a time: `cd 中文` answers "no
// such file or directory" for a directory that is there. The name picked is one
// the system actually has, in the user's own language where it is offered.
func TestPickUTF8Locale(t *testing.T) {
	tests := []struct {
		name      string
		preferred string
		available []string
		want      string
	}{
		{"the user's own", "zh_CN.UTF-8", []string{"en_US.UTF-8", "zh_CN.UTF-8"}, "zh_CN.UTF-8"},
		{"the user's own, missing", "zh_CN.UTF-8", []string{"en_US.UTF-8"}, "en_US.UTF-8"},
		{"no preference", "", []string{"C.UTF-8", "en_US.UTF-8"}, "en_US.UTF-8"},
		{"a minimal system", "", []string{"C.UTF-8", "C"}, "C.UTF-8"},
		{"glibc's spelling", "", []string{"C.utf8"}, "C.utf8"},
		{"the user's own, glibc's spelling", "zh_CN.UTF-8", []string{"zh_CN.utf8"}, "zh_CN.utf8"},
		{"none at all", "", []string{"C", "POSIX"}, ""},
		{"nothing to ask", "", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickUTF8Locale(tc.preferred, tc.available); got != tc.want {
				t.Errorf("pickUTF8Locale(%q, %q) = %q, want %q", tc.preferred, tc.available, got, tc.want)
			}
		})
	}
}

// A locale the environment already names is the one used, empty values
// included: a unit that cleared LANG says no more than launchd, and both are a
// reason to name one.
func TestNamesLocale(t *testing.T) {
	tests := []struct {
		env  []string
		want bool
	}{
		{[]string{"PATH=/bin"}, false},
		{[]string{"LANG="}, false},
		{[]string{"LC_ALL="}, false},
		{[]string{"LANG=en_US.UTF-8"}, true},
		{[]string{"LC_CTYPE=UTF-8"}, true},
		{[]string{"LC_ALL=C"}, true},
		{[]string{"LANGX=en_US.UTF-8"}, false},
	}
	for _, tc := range tests {
		if got := namesLocale(tc.env); got != tc.want {
			t.Errorf("namesLocale(%q) = %v, want %v", tc.env, got, tc.want)
		}
	}
}

// The environment a shell is started with names a UTF-8 locale when this
// process's does not — which is what a window opened from the Dock hands it.
func TestShellEnvNamesAUTF8LocaleWhereThisOneNamesNone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a ConPTY needs no locale")
	}
	if utf8Locale() == "" {
		t.Skip("this system offers no UTF-8 locale to name")
	}
	t.Setenv("LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")

	env := shellEnv()
	got := ""
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "LANG="); ok {
			got = v
		}
	}
	if want := utf8Locale(); got != want {
		t.Fatalf("LANG in the shell's environment = %q, want %q", got, want)
	}
}

// A locale this process names is kept, so a daemon started from a terminal — or
// on purpose as `LANG=C tfd` — behaves as its environment asked.
func TestShellEnvKeepsTheLocaleItWasGiven(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "en_GB.ISO8859-1")

	env := shellEnv()
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "LANG="); ok && v != "en_GB.ISO8859-1" {
			t.Fatalf("LANG = %q, want the one this process was given", v)
		}
	}
}

// The shell itself sees the locale: the environment is not just built, it is the
// one the program is started with.
func TestAShellSeesTheUTF8Locale(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a ConPTY needs no locale")
	}
	if utf8Locale() == "" {
		t.Skip("this system offers no UTF-8 locale to name")
	}
	t.Setenv("LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")

	sess, err := Start("locale", 80, 24, "", "sh")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()

	if err := sess.Write([]byte(`printf '\n<%s>\n' "$LANG"` + "\r")); err != nil {
		t.Fatalf("type the prompt: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		out, _, _ := sess.Ring().ReadFrom(0)
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") {
				// glibc spells the name "en_US.utf8" where macOS says
				// "en_US.UTF-8"; both are the locale that was asked for.
				if sameLocale(line[1:len(line)-1], utf8Locale()) {
					return
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	out, _, _ := sess.Ring().ReadFrom(0)
	t.Fatalf("the shell ran without a UTF-8 locale: %q", out)
}
