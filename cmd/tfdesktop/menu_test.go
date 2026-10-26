package main

import (
	"runtime"
	"testing"

	"github.com/egoist/mygo"

	"github.com/CarlOpenLab/beacon/locales"
)

// The menu bar is built at startup, before anything else runs, so it must not
// reach into the updater plugin: updater.MenuItem() asks a plugin that needs the
// app's event loop and would panic or hang on the main thread's queue (see
// menu.go). Replacing the item is how the test proves appMenu asked for it
// without it.
func TestAppMenuAsksForTheUpdaterItemWithoutThePlugin(t *testing.T) {
	old := updaterMenuItem
	t.Cleanup(func() { updaterMenuItem = old })

	asked := 0
	updaterMenuItem = func() *mygo.MenuItem {
		asked++
		return &mygo.MenuItem{Label: locales.UpdateCheckNow()}
	}

	if m := appMenu(&window{}); m == nil {
		t.Fatal("appMenu built no menu")
	}
	if asked != 1 {
		t.Errorf("appMenu asked for the %s item %d times, want 1", locales.UpdateCheckNow(), asked)
	}
}

// appMenu is only worth having if something installs it: on macOS the bar mygo
// draws by default has no room for 检查更新…, and the app called appMenu from
// nowhere. This fails if that wiring is dropped again.
func TestInstallMenuPutsTheUpdaterItemOnTheBar(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("mygo draws a menu bar on macOS only")
	}

	old := updaterMenuItem
	t.Cleanup(func() { updaterMenuItem = old })
	updaterMenuItem = func() *mygo.MenuItem {
		return &mygo.MenuItem{Label: locales.UpdateCheckNow()}
	}

	installMenu(&window{})

	m := mygo.App.Menu()
	if m == nil {
		t.Fatal("installMenu left the app without a menu")
	}
	if !menuHasLabel(m.Items(), locales.UpdateCheckNow()) {
		t.Errorf("installed menu bar has no %s item: %v", locales.UpdateCheckNow(), labelsOf(m.Items()))
	}
}

func menuHasLabel(items []*mygo.MenuItem, label string) bool {
	for _, it := range items {
		if it.Label == label || menuHasLabel(it.Submenu, label) {
			return true
		}
	}
	return false
}

func labelsOf(items []*mygo.MenuItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Label)
	}
	return out
}
