package session

import (
	"errors"
	"strings"
	"testing"
)

// fakeLook resolves only the names that are "installed".
func fakeLook(installed map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if path, ok := installed[name]; ok {
			return path, nil
		}
		return "", errors.New("executable file not found in %PATH%")
	}
}

func TestPickWindowsShellPrefersPowerShell7(t *testing.T) {
	look := fakeLook(map[string]string{
		"pwsh":       `C:\Program Files\PowerShell\7\pwsh.exe`,
		"powershell": `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
	})
	if got := pickWindowsShell(look, `C:\Windows\system32\cmd.exe`); got != `C:\Program Files\PowerShell\7\pwsh.exe` {
		t.Fatalf("pickWindowsShell = %q, want pwsh", got)
	}
}

func TestPickWindowsShellFallsBackToWindowsPowerShell(t *testing.T) {
	// PowerShell 7 is not installed; Windows PowerShell still is.
	look := fakeLook(map[string]string{
		"powershell": `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
	})
	if got := pickWindowsShell(look, `C:\Windows\system32\cmd.exe`); got != `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe` {
		t.Fatalf("pickWindowsShell = %q, want Windows PowerShell", got)
	}
}

func TestPickWindowsShellPrefersPowerShellOverComSpec(t *testing.T) {
	// COMSPEC is always set on Windows. It must not win over PowerShell.
	look := fakeLook(map[string]string{"pwsh": "pwsh.exe"})
	if got := pickWindowsShell(look, `C:\Windows\system32\cmd.exe`); got != "pwsh.exe" {
		t.Fatalf("pickWindowsShell = %q, want pwsh.exe", got)
	}
}

func TestPickWindowsShellFindsPowerShellOutsidePATH(t *testing.T) {
	// LookPath resolves a path with a separator directly, so a PowerShell
	// that is installed but missing from PATH still wins.
	const abs = `C:\Program Files\PowerShell\7\pwsh.exe`
	look := fakeLook(map[string]string{abs: abs})
	if got := pickWindowsShell(look, `C:\Windows\system32\cmd.exe`); got != abs {
		t.Fatalf("pickWindowsShell = %q, want the absolute pwsh path", got)
	}
}

func TestPickWindowsShellLastResortsToComSpec(t *testing.T) {
	look := fakeLook(nil)
	if got := pickWindowsShell(look, `C:\Windows\system32\cmd.exe`); got != `C:\Windows\system32\cmd.exe` {
		t.Fatalf("pickWindowsShell = %q, want COMSPEC", got)
	}
}

func TestPickWindowsShellNeverEmpty(t *testing.T) {
	look := fakeLook(nil)
	if got := pickWindowsShell(look, ""); got == "" {
		t.Fatal("pickWindowsShell returned an empty shell")
	}
}

func TestIsPowerShell(t *testing.T) {
	for _, path := range []string{
		`C:\Program Files\PowerShell\7\pwsh.exe`,
		`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
		"pwsh",
		"powershell.exe",
		`C:\tools\PWSH.EXE`,
	} {
		if !isPowerShell(path) {
			t.Errorf("isPowerShell(%q) = false, want true", path)
		}
	}
	for _, path := range []string{
		`C:\Windows\System32\cmd.exe`,
		`C:\Program Files\Git\bin\bash.exe`,
		"",
	} {
		if isPowerShell(path) {
			t.Errorf("isPowerShell(%q) = true, want false", path)
		}
	}
}

// A PowerShell shell is started with the prompt wrapper that keeps its
// process directory in step with where it stands; other shells are started
// as they always were.
func TestShellArgs(t *testing.T) {
	args := shellArgs(`C:\Program Files\PowerShell\7\pwsh.exe`)
	if len(args) != 3 || args[0] != "-NoExit" || args[1] != "-Command" {
		t.Fatalf("shellArgs(pwsh) = %q, want -NoExit -Command <hook>", args)
	}
	if !strings.Contains(args[2], "$global:__beacon_prompt") {
		t.Fatalf("shellArgs(pwsh) hook = %q, want the prompt wrapper", args[2])
	}
	if got := shellArgs(`C:\Windows\System32\cmd.exe`); got != nil {
		t.Fatalf("shellArgs(cmd) = %q, want nil", got)
	}
}
