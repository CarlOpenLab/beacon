package session

import (
	"errors"
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
