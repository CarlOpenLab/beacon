package main

import (
	"testing"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/locales"
)

// A session list is what opens tabs, and it must not open one for a shell that
// has ended: a tab is opened to watch something, and an ended shell is the one
// thing left with nothing to watch. This is what used to put a 已退出 tab back
// on screen a moment after it was closed — the next list re-opened it, so the
// tab could not be got rid of at all.
func TestASessionListNeverOpensATabForAnEndedShell(t *testing.T) {
	w, _ := newTestShellWindow(t)
	w.fleet = newTestFleet(t)

	w.reconcile([]protocol.Summary{{SessionID: "s9", Alive: false, Title: "gone"}})
	if len(w.tabs) != 0 {
		t.Fatalf("a session list opened %d tabs for a shell that has ended", len(w.tabs))
	}
	if len(w.order) != 0 {
		t.Fatalf("order = %v, want nothing open", w.order)
	}

	// And a tab already watching a shell that ends keeps it: the ending is the
	// exit frame's to report, and a list that still names the session is not
	// allowed to undo it while its last output is being read.
	w2, _ := newTestShellWindow(t, "gone")
	w2.fleet = newTestFleet(t)
	tb := w2.tabs[1]
	tb.alive = false
	tb.finish(0)
	w2.reconcile([]protocol.Summary{{SessionID: tb.id, Alive: false, Title: "gone"}})
	if len(w2.tabs) != 1 {
		t.Fatalf("a session list took away the tab of a shell that ended: %d tabs", len(w2.tabs))
	}
}

func TestLocalRosterKeepsRemoteTabs(t *testing.T) {
	w, _ := newTestShellWindow(t, "remote")
	w.tabs[1].machine = "other-machine"
	w.reconcile(nil)
	if len(w.order) != 1 || w.tabs[1] == nil {
		t.Fatal("the local roster removed a remote terminal")
	}
}

func TestClosedRemoteTabDisappearsFromTheSessionList(t *testing.T) {
	w, _ := newTestShellWindow(t, "remote")
	tb := w.tabs[1]
	tb.machine = "other-machine"
	w.machineSel = tb.machine
	w.remoteSessions = map[string][]protocol.Summary{
		tb.machine: {{SessionID: tb.id, Title: "remote", Alive: true}},
	}
	w.removeTab(tb)
	w.reconcile(nil)
	if entries := w.sessionEntries(); len(entries) != 0 {
		t.Fatal("the closed remote terminal is still listed")
	}
}

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

func TestTabNamePrefersTheShellTitle(t *testing.T) {
	tb := &tab{id: "s3", title: "carl@mac: ~/code", program: "zsh", ordinal: 2}
	if got := tb.name(); got != "carl@mac: ~/code" {
		t.Fatalf("name = %q, want the title", got)
	}
}

func TestTabNameNamesTheShellBeforeATitleArrives(t *testing.T) {
	// A fresh tab used to read "s1", which says nothing about what it is.
	tb := &tab{id: "s1", program: "zsh", ordinal: 1}
	if got := tb.name(); got != "zsh 1" {
		t.Fatalf("name = %q, want %q", got, "zsh 1")
	}
}

func TestTabNameDisambiguatesShellsOfTheSameKind(t *testing.T) {
	first := &tab{id: "s1", program: "zsh", ordinal: 1}
	second := &tab{id: "s5", program: "zsh", ordinal: 2}
	if first.name() == second.name() {
		t.Fatalf("two shells got the same name %q", first.name())
	}
}

func TestTabNameFallsBackWhenTheShellIsUnknown(t *testing.T) {
	tb := &tab{id: "s2", ordinal: 2}
	if got := tb.name(); got != locales.TabOrdinal(2) {
		t.Fatalf("name = %q, want %q", got, locales.TabOrdinal(2))
	}
}

// A tab of a shell that has ended is named the same as any other tab, and the
// state it is in is on the dot, the dimming and the tooltip. Appending the state
// to the name — "zsh 1 — 已退出" — reads well in a test and not in the strip: the
// pill is a fixed 230 wide, so the word the suffix adds is the first thing the
// ellipsis takes away, and the tab ends up reading "zsh 1 — …".
func TestATabOfAnEndedShellIsNamedLikeAnyOtherAndKeepsTheCodeForTheTooltip(t *testing.T) {
	tb := &tab{id: "s1", program: "zsh", ordinal: 1, done: true, exit: 7}
	if got := tb.name(); got != "zsh 1" {
		t.Fatalf("name = %q, want %q", got, "zsh 1")
	}
	if got := tb.exitLine(); got != locales.TabExitedCode(7) {
		t.Fatalf("exitLine = %q, want the code", got)
	}
	// A shell that has not ended has nothing to say about an exit.
	live := &tab{id: "s2", program: "zsh", ordinal: 2}
	if got := live.exitLine(); got != "" {
		t.Fatalf("exitLine = %q for a shell that is still running, want nothing", got)
	}
}

func TestTabNameNeverShowsBareSessionID(t *testing.T) {
	// When program and ordinal were unassigned, tabs must never show raw "s1" or "s3".
	s1 := &tab{id: "s1"}
	if got := s1.name(); got != locales.TabOrdinal(1) {
		t.Fatalf("s1.name() = %q, want %q", got, locales.TabOrdinal(1))
	}
	s3 := &tab{id: "s3"}
	if got := s3.name(); got != locales.TabOrdinal(3) {
		t.Fatalf("s3.name() = %q, want %q", got, locales.TabOrdinal(3))
	}
}

func TestNextOrdinalRecyclesUnusedIndices(t *testing.T) {
	w := &window{tabs: make(map[int]*tab)}
	if got := w.nextOrdinal(machineLocal); got != 1 {
		t.Fatalf("first ordinal = %d, want 1", got)
	}
	w.tabs[1] = &tab{machine: machineLocal, ordinal: 1}
	if got := w.nextOrdinal(machineLocal); got != 2 {
		t.Fatalf("second ordinal = %d, want 2", got)
	}
	w.tabs[2] = &tab{machine: machineLocal, ordinal: 2}
	// After removing tab 1, index 1 is recycled
	delete(w.tabs, 1)
	if got := w.nextOrdinal(machineLocal); got != 1 {
		t.Fatalf("recycled ordinal = %d, want 1", got)
	}
}
