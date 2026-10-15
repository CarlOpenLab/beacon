package main

import "testing"

func TestProgramName(t *testing.T) {
	cases := map[string]string{
		"/bin/zsh":                    "zsh",
		"/opt/homebrew/bin/fish":      "fish",
		"/bin/bash":                   "bash",
		`C:\Windows\System32\cmd.exe`: "cmd",
		`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`: "PowerShell",
		`C:\Program Files\PowerShell\7\pwsh.exe`:                    "pwsh",
		"powershell.exe":                                            "PowerShell",
		"":                                                          "",
	}
	for path, want := range cases {
		if got := programName(path); got != want {
			t.Errorf("programName(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestTabLabelPrefersTheShellTitle(t *testing.T) {
	tb := &tab{id: "s3", title: "carl@mac: ~/code", program: "zsh", ordinal: 2}
	if got := tb.label(); got != "carl@mac: ~/code" {
		t.Fatalf("label = %q, want the title", got)
	}
}

func TestTabLabelNamesTheShellBeforeATitleArrives(t *testing.T) {
	// A fresh tab used to read "s1", which says nothing about what it is.
	tb := &tab{id: "s1", program: "zsh", ordinal: 1}
	if got := tb.label(); got != "zsh 1" {
		t.Fatalf("label = %q, want %q", got, "zsh 1")
	}
}

func TestTabLabelDisambiguatesShellsOfTheSameKind(t *testing.T) {
	first := &tab{id: "s1", program: "zsh", ordinal: 1}
	second := &tab{id: "s5", program: "zsh", ordinal: 2}
	if first.label() == second.label() {
		t.Fatalf("two shells got the same label %q", first.label())
	}
}

func TestTabLabelFallsBackWhenTheShellIsUnknown(t *testing.T) {
	tb := &tab{id: "s2", ordinal: 2}
	if got := tb.label(); got != "终端 2" {
		t.Fatalf("label = %q, want %q", got, "终端 2")
	}
}

func TestTabLabelStillReportsTheExitCode(t *testing.T) {
	tb := &tab{id: "s1", program: "zsh", ordinal: 1, done: true, exit: 7}
	if got := tb.label(); got != "zsh 1 (7)" {
		t.Fatalf("label = %q, want %q", got, "zsh 1 (7)")
	}
}
