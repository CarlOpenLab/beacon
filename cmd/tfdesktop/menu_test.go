package main

import (
	"testing"

	"github.com/egoist/mygo"
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
		return &mygo.MenuItem{Label: "检查更新…"}
	}

	if m := appMenu(&window{}); m == nil {
		t.Fatal("appMenu built no menu")
	}
	if asked != 1 {
		t.Errorf("appMenu asked for the 检查更新… item %d times, want 1", asked)
	}
}
