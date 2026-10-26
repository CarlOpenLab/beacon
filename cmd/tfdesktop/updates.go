package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/locales"
)

// updatePrefs is what the 更新 pane shows: whether this build can update
// itself at all, the two switches, and when it last asked.
type updatePrefs struct {
	available          bool
	automaticChecks    bool
	automaticDownloads bool
	lastCheck          time.Time
}

// These three calls go through variables rather than straight into mygo's
// updater plugin, for the reason menu.go gives about updaterMenuItem: the
// plugin is registered by main, and asking it anything needs the app's event
// loop. A test process has neither, so a direct call either panics or blocks on
// the main thread's queue. Tests replace them; the window uses the real ones.
var (
	loadUpdatePrefs = func() updatePrefs {
		// A build that cannot update itself has nothing to show but the reason:
		// Enabled is false for development builds and for apps a package manager
		// installed, both of which the plugin refuses to touch.
		if !mygo.Updater.Enabled() {
			return updatePrefs{}
		}
		return updatePrefs{
			available:          true,
			automaticChecks:    updater.AutomaticChecks(),
			automaticDownloads: updater.AutomaticDownloads(),
			lastCheck:          updater.LastCheck(),
		}
	}
	saveUpdatePrefs = func(automaticChecks, automaticDownloads bool) {
		updater.SetAutomaticChecks(automaticChecks)
		updater.SetAutomaticDownloads(automaticDownloads)
	}
	checkForUpdatesNow = func() { updater.CheckForUpdates() }
)

// versionLabel is the running version as the 更新 pane prints it: "v0.3.0" for a
// packaged build, "vdev" for one that was only ever run from a checkout.
func versionLabel() string {
	if v := strings.TrimSpace(mygo.App.Version()); v != "" {
		return "v" + v
	}
	return "vdev"
}

// lastCheckLabel says when the app last asked about updates.
func lastCheckLabel(when time.Time) string {
	if when.IsZero() {
		return locales.UpdateNever()
	}
	return when.Local().Format("2006-01-02 15:04")
}

// updatesPane draws the 更新 pane. What it shows is the updater plugin's own
// state, and flipping a switch writes that plugin's updater.json straight away:
// this pane is not part of the draft the other panes edit, so 应用 and 恢复默认
// leave it alone.
//
// A build that cannot update itself gets the reason instead of switches — a
// control that cannot do anything is worse than a sentence saying so.
func (s *settingsWindow) updatesPane(c *ui.Context) {
	t := c.Theme()
	pal := paletteFor(c)
	prefs := loadUpdatePrefs()

	settingsFormGroup(c, pal, func() {
		ui.Form(c, func() {
			ui.Field(c, locales.UpdateCurrent(), func() { ui.Text(c, versionLabel()) })
			ui.Field(c, locales.UpdateLastCheck(), func() { ui.Text(c, lastCheckLabel(prefs.lastCheck)) })

			if !prefs.available {
				ui.Field(c, locales.UpdateAutoCheck(), func() { ui.Text(c, locales.UpdateUnavailable()) })
				ui.Field(c, locales.UpdateAutoDownload(), func() { ui.Text(c, locales.UpdateUnavailable()) })
			} else {
				// Each switch is drawn from the plugin's state and pushed back when
				// clicked, so the pane reads what is on disk every frame and keeps no
				// second copy of it.
				checks := prefs.automaticChecks
				ui.Field(c, locales.UpdateAutoCheck(), func() {
					if ui.Switch(c, &checks).AlignSelf(ui.End).Clicked() {
						saveUpdatePrefs(checks, prefs.automaticDownloads)
					}
				})
				downloads := prefs.automaticDownloads
				ui.Field(c, locales.UpdateAutoDownload(), func() {
					if ui.Switch(c, &downloads).AlignSelf(ui.End).Clicked() {
						saveUpdatePrefs(prefs.automaticChecks, downloads)
					}
				})
			}
			paneNote(c, locales.UpdateDailyNote(), t.TextMuted)
			// A build that cannot update itself gets the reason instead of a button —
			// the same rule the two switches above follow.
			if prefs.available {
				ui.Field(c, "", func() {
					if ui.Button(c, locales.UpdateCheckNow()).AlignSelf(ui.End).Clicked() {
						checkForUpdatesNow()
					}
				})
				paneNote(c, locales.UpdateFoundNote(), t.TextMuted)
			} else {
				paneNote(c, locales.UpdateUnavailableNote(), t.TextMuted)
			}
			paneNote(c, locales.UpdateSourceNote(), t.TextMuted)
		})
	})
}
