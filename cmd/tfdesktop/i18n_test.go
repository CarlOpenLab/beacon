package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/locales"
)

// The catalogs are the whole of the switch: every message the window says has
// an English form, and the accessors read it by tag. This is what the language
// control in the settings window moves between — the app's own locale, which
// only a running app can change, is mygo's business.
func TestEveryMessageHasAnEnglishForm(t *testing.T) {
	zh := locales.In("zh-Hans")
	en := locales.In("en")
	pairs := []struct {
		zh, en string
	}{
		{zh.SettingsFont(), en.SettingsFont()},
		{zh.SettingsFontSize(), en.SettingsFontSize()},
		{zh.SettingsLanguage(), en.SettingsLanguage()},
		{zh.SettingsPaneTerminal(), en.SettingsPaneTerminal()},
		{zh.SettingsPaneRelay(), en.SettingsPaneRelay()},
		{zh.SettingsPaneServer(), en.SettingsPaneServer()},
		{zh.SettingsPaneUpdates(), en.SettingsPaneUpdates()},
		{zh.SettingsRelayUrl(), en.SettingsRelayUrl()},
		{zh.SettingsServer(), en.SettingsServer()},
		{zh.ActionNewTerminalTip(), en.ActionNewTerminalTip()},
		{zh.TabRename(), en.TabRename()},
		{zh.StateReadonly(), en.StateReadonly()},
		{zh.ComposerPlaceholder(), en.ComposerPlaceholder()},
		{zh.EmptyTitle(), en.EmptyTitle()},
		{zh.LeaseWho("ada"), en.LeaseWho("ada")},
		{zh.TabOrdinal(2), en.TabOrdinal(2)},
		{zh.ActionHideMachine(), en.ActionHideMachine()},
		{zh.SidebarOthers(3), en.SidebarOthers(3)},
		{zh.MachineIconMenu(), en.MachineIconMenu()},
		{zh.MachineIconLaptop(), en.MachineIconLaptop()},
		{zh.MachineIconMonitor(), en.MachineIconMonitor()},
		{zh.MachineIconServer(), en.MachineIconServer()},
		{zh.MachineIconHardDrive(), en.MachineIconHardDrive()},
		{zh.MachineIconCpu(), en.MachineIconCpu()},
		{zh.MachineIconDatabase(), en.MachineIconDatabase()},
		{zh.MachineIconCloud(), en.MachineIconCloud()},
		{zh.MachineIconContainer(), en.MachineIconContainer()},
		{zh.MachineIconRouter(), en.MachineIconRouter()},
		{zh.MachineIconPrinter(), en.MachineIconPrinter()},
		{zh.MachineIconBox(), en.MachineIconBox()},
		{zh.MachineIconTerminal(), en.MachineIconTerminal()},
		{zh.SidebarNoneVisible(), en.SidebarNoneVisible()},
		{zh.SidebarRestoreMachines(), en.SidebarRestoreMachines()},
		{zh.SettingsPaneSidebar(), en.SettingsPaneSidebar()},
		{zh.SettingsSidebarMachines(), en.SettingsSidebarMachines()},
		{zh.SettingsSidebarNote(), en.SettingsSidebarNote()},
		{zh.SettingsSidebarToggle("mac-mini"), en.SettingsSidebarToggle("mac-mini")},
	}
	for _, p := range pairs {
		if p.zh == p.en {
			t.Errorf("%q has no English form", p.zh)
		}
	}
	// A message with an argument keeps its argument in the other language.
	if got := en.TabOrdinal(2); got != "Terminal 2" {
		t.Errorf("TabOrdinal(2) in English = %q, want %q", got, "Terminal 2")
	}
	if got := en.LeaseWho("ada"); got != "ada is controlling this terminal" {
		t.Errorf("LeaseWho(\"ada\") in English = %q", got)
	}
	// The source language is what a first run and a missing tag both give.
	if got := locales.In("").SettingsFont(); got != zh.SettingsFont() {
		t.Errorf("the source language says %q, want %q", got, zh.SettingsFont())
	}
}

// The language control writes what was picked, like every other field: the
// window applies it on the click, and the store keeps it for the next launch.
func TestTheLanguageControlWritesTheStore(t *testing.T) {
	sw, store := newTestSettingsWindow(t)

	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()
	if err := tt.Click(locales.SettingsLangEn()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsLangEn(), err)
	}
	tt.Frame()

	if got := store.Get().Language; got != "en" {
		t.Fatalf("stored Language = %q, want %q", got, "en")
	}
	if sw.language != "en" {
		t.Fatalf("the window's draft holds %q, want %q", sw.language, "en")
	}
}

// Following the system is the default, and anything the file says that is not a
// language the app is written in maps onto it: a hand-edited settings file must
// not leave the shell in a language it cannot show.
func TestAnUnknownLanguageFollowsTheSystem(t *testing.T) {
	for _, tag := range []string{"", "zh-Hans", "en"} {
		if got := normalizeLanguage(tag); got != tag {
			t.Errorf("normalizeLanguage(%q) = %q, want it kept", tag, got)
		}
	}
	for _, tag := range []string{"fr", "zh-Hant", "  en  "} {
		want := ""
		if tag == "  en  " {
			want = "en"
		}
		if got := normalizeLanguage(tag); got != want {
			t.Errorf("normalizeLanguage(%q) = %q, want %q", tag, got, want)
		}
	}
	if got := languageIndex("en"); got != 2 {
		t.Errorf("languageIndex(\"en\") = %d, want the English entry", got)
	}
	if got := languageIndex("nonsense"); got != 0 {
		t.Errorf("languageIndex(\"nonsense\") = %d, want the follow-the-system entry", got)
	}
}
