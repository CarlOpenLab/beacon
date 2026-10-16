package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
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
func appMenu(w *window) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "term-fleet", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			updaterMenuItem(),
			mygo.Separator(),
			{
				Label:       "设置…",
				Accelerator: "CmdOrCtrl+,",
				Click:       func(*mygo.MenuItem, *mygo.Window) { go w.openSettings() },
			},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleWindowMenu},
	})
}
