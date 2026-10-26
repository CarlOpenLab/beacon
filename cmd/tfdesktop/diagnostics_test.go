package main

// 设置 → 诊断 is the page a person opens after a blank pane, so it has to draw
// with a real log behind it: the file it names, the level it records at, and the
// three buttons that get it out of the app.

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/diag"
	"github.com/CarlOpenLab/beacon/locales"
)

func TestTheDiagnosticsPageNamesTheLog(t *testing.T) {
	logger, err := diag.Open(t.TempDir(), "test", diag.Debug)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		_ = logger.Close()
		diag.SetDefault(nil)
	})
	diag.SetDefault(logger)

	sw, _ := newTestSettingsWindow(t)
	sw.pane = paneDiagnostics
	tt := ui.NewTester(sw.view, 720, 560)
	tt.Frame()

	for _, want := range []string{
		locales.SettingsPaneDiagnostics(),
		locales.SettingsDiagFile(),
		locales.SettingsDiagCopy(),
		locales.SettingsDiagOpen(),
		locales.SettingsDiagSave(),
		locales.SettingsDiagLevel(),
		locales.SettingsDiagLevelDebug(),
	} {
		if !tt.HasText(want) {
			t.Fatalf("the diagnostics page does not show %q: %q", want, tt.Texts())
		}
	}
}
