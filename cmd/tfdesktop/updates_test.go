package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/locales"
)

// updateStub replaces the three calls the 更新 pane makes into mygo's updater
// plugin with fakes that record what the pane asked for. The real ones need the
// plugin's event loop, which a test process does not have — that is the reason
// updates.go keeps them in variables in the first place.
type updateStub struct {
	prefs  updatePrefs
	saves  int
	checks int
}

func stubUpdates(t *testing.T, prefs updatePrefs) *updateStub {
	t.Helper()
	st := &updateStub{prefs: prefs}
	load, save, check := loadUpdatePrefs, saveUpdatePrefs, checkForUpdatesNow
	t.Cleanup(func() { loadUpdatePrefs, saveUpdatePrefs, checkForUpdatesNow = load, save, check })

	loadUpdatePrefs = func() updatePrefs { return st.prefs }
	saveUpdatePrefs = func(automaticChecks, automaticDownloads bool) {
		st.saves++
		st.prefs.automaticChecks, st.prefs.automaticDownloads = automaticChecks, automaticDownloads
	}
	checkForUpdatesNow = func() { st.checks++ }
	return st
}

// openUpdatesPane opens the settings window on the 更新 pane.
func openUpdatesPane(t *testing.T, sw *settingsWindow) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(sw.view, 720, 560)
	tt.Frame()
	if err := tt.Click(locales.SettingsPaneUpdates()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsPaneUpdates(), err)
	}
	tt.Frame()
	return tt
}

// clickAndSettle clicks a label and lets the click land: a click on a field's
// label reaches the control it names through the runtime's queue, so the toggle
// can take more than the frame of the click to show up.
func clickAndSettle(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("Click(%s): %v", label, err)
	}
	for range 4 {
		tt.Frame()
	}
}

func TestUpdatesPaneShowsTheVersionAndThePluginSwitches(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	last := time.Date(2026, 10, 16, 9, 20, 0, 0, time.Local)
	stub := stubUpdates(t, updatePrefs{available: true, lastCheck: last})

	tt := openUpdatesPane(t, sw)
	if !tt.HasText(versionLabel()) {
		t.Errorf("the pane does not show the running version %q: %q", versionLabel(), tt.Texts())
	}
	if !tt.HasText(lastCheckLabel(last)) {
		t.Errorf("the pane does not show the last check (%s): %q", lastCheckLabel(last), tt.Texts())
	}
	for _, label := range []string{locales.UpdateAutoCheck(), locales.UpdateAutoDownload()} {
		if _, ok := tt.Find(label); !ok {
			t.Errorf("%s is missing from an updatable build's pane", label)
		}
	}

	if err := tt.Click(locales.UpdateCheckNow()); err != nil {
		t.Fatalf("Click(%s): %v", locales.UpdateCheckNow(), err)
	}
	tt.Frame()
	if stub.checks != 1 {
		t.Errorf("checking for updates asked the plugin %d times, want 1", stub.checks)
	}

	clickAndSettle(t, tt, locales.UpdateAutoCheck())
	if stub.saves != 1 || !stub.prefs.automaticChecks || stub.prefs.automaticDownloads {
		t.Errorf("toggling the automatic check wrote %+v after %d saves, want checks on and downloads off",
			stub.prefs, stub.saves)
	}

	clickAndSettle(t, tt, locales.UpdateAutoDownload())
	if !stub.prefs.automaticChecks || !stub.prefs.automaticDownloads {
		t.Errorf("toggling the automatic download wrote %+v, want both on", stub.prefs)
	}
}

// A .deb is updated by the package manager and a development build by whoever
// is writing it, so mygo refuses to update both: the pane says why, and offers
// no switch that would do nothing.
func TestUpdatesPaneExplainsABuildThatCannotUpdateItself(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	stub := stubUpdates(t, updatePrefs{})

	tt := openUpdatesPane(t, sw)
	if !tt.HasText(locales.UpdateNever()) {
		t.Errorf("an unchecked build should say so: %q", tt.Texts())
	}
	if !tt.HasText(locales.UpdateUnavailable()) {
		t.Errorf("the pane does not mark the switches as unavailable: %q", tt.Texts())
	}
	if _, ok := tt.Find(locales.UpdateCheckNow()); ok {
		t.Error("checking for updates should not be offered by a build that cannot update itself")
	}
	for _, label := range []string{locales.UpdateAutoCheck(), locales.UpdateAutoDownload()} {
		clickAndSettle(t, tt, label)
	}
	if stub.saves != 0 || stub.checks != 0 {
		t.Errorf("an unavailable build wrote to the plugin: %d saves, %d checks", stub.saves, stub.checks)
	}
}

func TestLastCheckLabel(t *testing.T) {
	if got := lastCheckLabel(time.Time{}); got != locales.UpdateNever() {
		t.Errorf("lastCheckLabel(zero) = %q", got)
	}
	when := time.Date(2026, 10, 16, 9, 20, 0, 0, time.Local)
	if got := lastCheckLabel(when); got != "2026-10-16 09:20" {
		t.Errorf("lastCheckLabel(%s) = %q", when, got)
	}
}
