package main

// 设置 → 诊断: the log this window keeps about itself, and the three things a
// person does with it when something on screen does not match the machines —
// find it, copy its path, send a copy.
//
// The page exists because the failure it is for has no error dialog: a pane
// that stays empty while its shell runs on. Nothing crashed, nothing to report,
// and the only evidence is the timeline in the file. A feature that is only
// reachable when everything works is the right place for it: it is read
// afterwards, not during.

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/diag"
	"github.com/CarlOpenLab/term-fleet/locales"
)

// diagnosticsPane draws the page: where the log is, what it records, and what
// the three buttons below do with it.
func (s *settingsWindow) diagnosticsPane(c *ui.Context) {
	t := c.Theme()
	logger := diag.Default()
	path := logger.Path()

	ui.Form(c, func() {
		ui.Field(c, locales.SettingsDiagFile(), func() {
			ui.Column(c).Gap(8).Children(func() {
				shown := path
				if shown == "" {
					shown = locales.SettingsDiagNoFile()
				}
				ui.Text(c, shown).FontSize(11.5).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, locales.SettingsDiagCopy()).Clicked() {
						c.Services().WriteClipboard(shown)
						s.notice(locales.SettingsDiagCopied())
					}
					if path != "" && ui.Button(c, locales.SettingsDiagOpen()).Clicked() {
						if err := revealInFileManager(path); err != nil {
							s.fail(locales.SettingsDiagOpenFailed(err.Error()))
						}
					}
					if path != "" && ui.Button(c, locales.SettingsDiagSave()).Clicked() {
						s.saveLogCopy()
					}
				})
			})
		}).Description(locales.SettingsDiagNote())

		ui.Field(c, locales.SettingsDiagLevel(), func() {
			label := locales.SettingsDiagLevelRegular()
			if logger.Level() == diag.Debug {
				label = locales.SettingsDiagLevelDebug()
			}
			ui.Text(c, label).FontSize(12.5).TextColor(t.Text)
		}).Description(locales.SettingsDiagLevelNote())
	})
}

// notice is the settings window saying something succeeded — copied, saved —
// in the same line that reports a save, so the feedback has one home instead
// of a second kind of message appearing somewhere else.
func (s *settingsWindow) notice(text string) {
	s.statusErr = false
	s.status = text
	s.savedAt = time.Now()
	if s.desk != nil {
		s.desk.invalidate()
	}
}

// saveLogCopy asks where to put a copy and writes it there. The dialog blocks,
// so it runs off the UI thread, and the answer comes back through the window's
// own update queue like every other change a goroutine makes.
func (s *settingsWindow) saveLogCopy() {
	logger := diag.Default()
	src := logger.Path()
	if src == "" {
		s.fail(locales.SettingsDiagNoFile())
		return
	}
	go func() {
		name := "term-fleet-log-" + time.Now().Format("20060102-150405") + ".log"
		defaultPath := name
		if home, err := os.UserHomeDir(); err == nil {
			defaultPath = filepath.Join(home, name)
		}
		dst, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title:       locales.SettingsDiagSaveTitle(),
			DefaultPath: defaultPath,
			Filters:     []mygo.FileFilter{{Name: "Log", Extensions: []string{"log", "txt"}}},
		})
		if err != nil || dst == "" {
			// A cancelled dialog is not a failure, and saying so would make the
			// page argue with a person who changed their mind.
			return
		}
		if err := copyFile(src, dst); err != nil {
			s.onDesk(func() { s.fail(locales.SettingsDiagSaveFailed(err.Error())) })
			return
		}
		s.onDesk(func() { s.notice(locales.SettingsDiagSaved(dst)) })
	}()
}

// onDesk runs fn on the window's thread, or directly when there is no window —
// a settings window drawn by a test has no event loop to post to.
func (s *settingsWindow) onDesk(fn func()) {
	if s.desk == nil {
		fn()
		return
	}
	s.desk.update(fn)
}

// copyFile writes src's bytes to dst, replacing it. It is one log file's worth
// of data, so reading it whole is the simple answer.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// revealInFileManager shows the log where the system keeps files: the Finder
// selecting it on macOS, Explorer on Windows, the directory on the platforms
// where a file manager is a command rather than an API.
func revealInFileManager(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", path).Start()
	case "windows":
		return exec.Command("explorer", "/select,"+path).Start()
	default:
		return exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}
