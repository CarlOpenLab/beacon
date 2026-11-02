package session

import "strings"

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

// powerShellPromptHook is a prompt wrapper a PowerShell shell is started with.
// PowerShell's current *location* is its own idea, not the process's working
// directory: Set-Location does not call SetCurrentDirectory, so the PEB keeps
// naming the directory the shell was started in, and a new tab that follows
// the shell would open at the top of the house forever. The wrapper draws the
// prompt the person's profile defined and then syncs the process's directory
// from the provider's, which is where liveCwd reads it from.
//
// The original prompt is called before the sync on purpose: a prompt that
// shows the last command's exit state reads $? when it begins, and a statement
// of ours running first would overwrite that. A location that is not a file
// system — the Registry provider, say — has no OS directory to sync, and is
// left alone.
const powerShellPromptHook = `$global:__beacon_prompt = $function:prompt; ` +
	`function global:prompt { & $global:__beacon_prompt; ` +
	`try { $p = $ExecutionContext.SessionState.Path.CurrentLocation; ` +
	`if ($p.Provider.Name -eq 'FileSystem') { ` +
	`[Environment]::CurrentDirectory = $p.ProviderPath } } catch {} }`

// shellArgs is what a shell is started with beyond its own startup: the
// prompt wrapper for PowerShell, and nothing for a shell whose process
// directory already follows its cd.
func shellArgs(path string) []string {
	if !isPowerShell(path) {
		return nil
	}
	return []string{"-NoExit", "-Command", powerShellPromptHook}
}

// isPowerShell reports whether a shell path names PowerShell, 7 or 5.1:
// "C:\Program Files\PowerShell\7\pwsh.exe" and "powershell.exe" both do. Both
// separators are handled because the path is a Windows one even when the test
// that asks runs elsewhere.
func isPowerShell(path string) bool {
	base := path
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(strings.ToLower(base), ".exe")
	return base == "pwsh" || base == "powershell"
}
