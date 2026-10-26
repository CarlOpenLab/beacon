package main

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"

	"github.com/CarlOpenLab/beacon/locales"
)

// updaterMenuItem is the 检查更新… item. It is a variable rather than a call so
// a test can replace it: updater.MenuItem() asks the plugin, which needs the
// app's event loop and panics or hangs without one — the same reason updates.go
// keeps its three calls in variables.
var updaterMenuItem = func() *mygo.MenuItem { return updater.MenuItem() }

// appMenu is the menu bar, and it is set on macOS only: that is the one platform
// where mygo draws one anyway, and its default menu has no room for 检查更新…
// The Edit and Window menus stay, because every tab is a real shell and copy,
// paste and select all reach it through the Edit roles as they do in any other
// app.
//
// Elsewhere the 更新 pane's 检查更新… button is the way in, and no menu bar
// appears that was not there before.
func installMenu(w *window) {
	if runtime.GOOS != "darwin" {
		return
	}
	mygo.App.SetMenu(appMenu(w))
}

// menuAccelFor is an action's accelerator in the form mygo's menus take, so a
// shortcut rebound in Settings is the one the menu shows.
func (w *window) menuAccelFor(id string) string {
	// While a shortcut is being recorded the menu must not eat the keys: a
	// person recording ⌘, is not asking for Settings to open again.
	if w.settingsContent != nil && w.settingsContent.recording != "" {
		return ""
	}
	if a, ok := actionByID(id); ok {
		return menuAccel(w.accel(a))
	}
	return ""
}

func appMenu(w *window) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Beacon", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			updaterMenuItem(),
			mygo.Separator(),
			{
				Label:       locales.MenuSettings(),
				Accelerator: w.menuAccelFor("settings.open"),
				Click: func(*mygo.MenuItem, *mygo.Window) {
					if w.win != nil {
						w.win.Update(w.openSettings)
					} else {
						w.openSettings()
					}
				},
			},
			// The log, one item from the menu bar: a person reporting a blank
			// pane should not have to be told a path first.
			{
				Label: locales.MenuDiagnostics(),
				Click: func(*mygo.MenuItem, *mygo.Window) {
					if w.win != nil {
						w.win.Update(func() { w.openSettingsAt(paneDiagnostics) })
					} else {
						w.openSettingsAt(paneDiagnostics)
					}
				},
			},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleWindowMenu},
	})
}
