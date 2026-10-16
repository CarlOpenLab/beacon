package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
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
		return "还没有检查过"
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
	prefs := loadUpdatePrefs()

	ui.Form(c, func() {
		ui.Field(c, "当前版本", func() { ui.Text(c, versionLabel()) })
		ui.Field(c, "上次检查", func() { ui.Text(c, lastCheckLabel(prefs.lastCheck)) })

		if !prefs.available {
			ui.Field(c, "自动检查更新", func() { ui.Text(c, "不可用") })
			ui.Field(c, "自动下载更新", func() { ui.Text(c, "不可用") })
		} else {
			// Each switch is drawn from the plugin's state and pushed back when
			// clicked, so the pane reads what is on disk every frame and keeps no
			// second copy of it.
			checks := prefs.automaticChecks
			ui.Field(c, "自动检查更新", func() {
				if ui.Switch(c, &checks).AlignSelf(ui.Start).Clicked() {
					saveUpdatePrefs(checks, prefs.automaticDownloads)
				}
			})
			downloads := prefs.automaticDownloads
			ui.Field(c, "自动下载更新", func() {
				if ui.Switch(c, &downloads).AlignSelf(ui.Start).Clicked() {
					saveUpdatePrefs(prefs.automaticChecks, downloads)
				}
			})
		}
	})
	ui.Text(c, "每天在后台问一次；打开自动下载后，新版本装好，下次启动就是新版本。").
		FontSize(11).TextColor(t.TextMuted)

	// A build that cannot update itself gets the reason instead of a button —
	// the same rule the two switches above follow.
	if prefs.available {
		if ui.Button(c, "检查更新…").Clicked() {
			checkForUpdatesNow()
		}
		ui.Text(c, "发现新版本时会弹出窗口，列出版本说明，可以安装、稍后提醒或跳过这一版。").
			FontSize(11).TextColor(t.TextMuted)
	} else {
		ui.Text(c, "这份构建不能自我更新：开发构建，或由包管理器（.deb 等）安装，更新交给它。"+
			"用安装器或 tar.gz 装的版本会自己更新。").
			FontSize(11).TextColor(t.TextMuted)
	}
	ui.Text(c, "更新源：github.com/CarlOpenLab/term-fleet 的正式发布版，包带签名校验。").
		FontSize(11).TextColor(t.TextMuted)
}
