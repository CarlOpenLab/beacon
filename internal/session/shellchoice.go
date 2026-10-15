package session

// windowsShellCandidates are the shells tried, in order, when a session was
// not told which one to run. PowerShell comes first and cmd.exe is only the
// last resort: cmd is not a good default for a terminal today, but it is
// always present, so it stays as the floor. The absolute paths cover
// installations that are missing from PATH.
var windowsShellCandidates = []string{
	"pwsh",
	"powershell",
	`C:\Program Files\PowerShell\7\pwsh.exe`,
	`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
}

// pickWindowsShell chooses the shell for a session that did not name one.
//
// look resolves a program name the way exec.LookPath does (and, given a path
// with a separator, checks that path directly). It is a parameter so the
// choice can be tested on any platform.
func pickWindowsShell(look func(string) (string, error), comspec string) string {
	for _, candidate := range windowsShellCandidates {
		if path, err := look(candidate); err == nil && path != "" {
			return path
		}
	}
	if comspec != "" {
		return comspec
	}
	return `C:\Windows\System32\cmd.exe`
}
