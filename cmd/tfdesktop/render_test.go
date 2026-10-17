package main

import (
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/settings"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

// The previews are how the shell is looked at: a test can assert that a label is
// on screen, but only a picture shows whether the window is worth looking at.

const (
	previewWidth  = 1180
	previewHeight = 720

	// The pane a shell gets at the preview size, in cells.
	previewCols = 108
	previewRows = 37
)

// TestRenderShellPreviews writes pictures of the shell into the directory named
// by TF_RENDER: both appearances, with a shell on screen and with none. It is
// skipped unless that variable is set, because it writes files and because what
// it makes can only be judged with eyes.
func TestRenderShellPreviews(t *testing.T) {
	dir := os.Getenv("TF_RENDER")
	if dir == "" {
		t.Skip("set TF_RENDER=<dir> to write the shell previews")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		mode  string
		shell bool
		taken bool
		fleet bool
		pick  bool
		ask   bool
		// onRemote puts another machine's shell on screen: its row in the 会话
		// panel is the active one, which is what a tab list has to show.
		onRemote bool
	}{
		{"shell-dark", themeDark, true, false, false, false, false, false},
		{"shell-light", themeLight, true, false, false, false, false, false},
		{"empty-dark", themeDark, false, false, false, false, false, false},
		{"empty-light", themeLight, false, false, false, false, false, false},
		// A shell another client holds: the glow around the terminal, the
		// popover under it, and the mark on its row.
		{"taken-dark", themeDark, true, true, false, false, false, false},
		{"taken-light", themeLight, true, true, false, false, false, false},
		// A fleet: the 机器 panel as a list you switch between, with this machine
		// and one other.
		{"fleet-dark", themeDark, true, false, true, false, false, false},
		{"fleet-light", themeLight, true, false, true, false, false, false},
		// Another machine picked: its row carries the selection, and the 会话 panel
		// below shows the shells that machine is running rather than this one's.
		{"fleet-picked-dark", themeDark, true, false, true, true, false, false},
		{"fleet-picked-light", themeLight, true, false, true, true, false, false},
		// Picking an unreachable machine: the dialog that asks the relay for a
		// short-lived credential first.
		{"fleet-ask-dark", themeDark, true, false, true, false, true, false},
		{"fleet-ask-light", themeLight, true, false, true, false, true, false},
		// The other machine's shell on screen: its row carries the accent bar the
		// picked machine carries, and this machine's shell is a tab behind it.
		{"fleet-remote-dark", themeDark, true, false, true, true, false, true},
		{"fleet-remote-light", themeLight, true, false, true, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := previewWindow(t, tc.mode, tc.shell)
			if tc.taken {
				w.tabs[1].setLease(protocol.RoleViewer, "c9", 4)
				w.leaseOpen = true
			}
			if tc.fleet {
				previewFleet(w)
			}
			if tc.pick {
				w.machineSel = "mac-studio"
			}
			if tc.ask {
				w.reachPrompt = "mac-studio"
			}
			if tc.onRemote {
				// The tab a click on that row leaves: the same picture, labelled as
				// the shell on the other machine.
				remote := &tab{
					id: "s7", machine: "mac-studio", title: "~/work/term-fleet",
					ordinal: 2, term: w.tabs[1].term, alive: true,
				}
				w.tabs[2] = remote
				w.order = append(w.order, 2)
				w.selectTab(remote)
			}
			tt := ui.NewTester(func(c *ui.Context) { w.view(c) }, previewWidth, previewHeight)
			out := filepath.Join(dir, tc.name+".png")
			file, err := os.Create(out)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := png.Encode(file, tt.Image()); err != nil {
				t.Fatal(err)
			}
			t.Logf("wrote %s", out)
		})
	}
}

// TestRenderSettingsPreviews writes a picture of every pane of the settings
// window, in both appearances, into the directory named by TF_RENDER. The panes
// are forms: their labels line up with each other, and that is something only a
// picture shows.
func TestRenderSettingsPreviews(t *testing.T) {
	dir := os.Getenv("TF_RENDER")
	if dir == "" {
		t.Skip("set TF_RENDER=<dir> to write the settings previews")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		mode  string
		pane  int
		label string
	}{
		{"settings-dark-terminal", themeDark, paneTerminal, "终端"},
		{"settings-dark-relay", themeDark, paneRelay, "远程"},
		{"settings-dark-server", themeDark, paneServer, "服务"},
		{"settings-dark-updates", themeDark, paneUpdates, "更新"},
		{"settings-dark-appearance", themeDark, paneAppearance, "外观"},
		{"settings-light-terminal", themeLight, paneTerminal, "终端"},
		{"settings-light-relay", themeLight, paneRelay, "远程"},
		{"settings-light-server", themeLight, paneServer, "服务"},
		{"settings-light-updates", themeLight, paneUpdates, "更新"},
		{"settings-light-appearance", themeLight, paneAppearance, "外观"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := settings.Open(t.TempDir())
			if _, err := store.Update(func(s *settings.Settings) { s.Theme = tc.mode }); err != nil {
				t.Fatal(err)
			}
			sw := newSettingsWindow(&window{}, store, newTestFleet(t))
			sw.pane = tc.pane
			tt := ui.NewTester(sw.view, 540, 440)
			out := filepath.Join(dir, tc.name+".png")
			file, err := os.Create(out)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := png.Encode(file, tt.Image()); err != nil {
				t.Fatal(err)
			}
			t.Logf("wrote %s", out)
		})
	}
}

// previewWindow is the window the previews draw: the appearance asked for, and,
// when a shell is wanted, one shell with a screenful of text in it. It has a
// real fleet behind it, so the machines' panel shows the machine it is on
// rather than a placeholder row.
func previewWindow(t *testing.T, mode string, shell bool) *window {
	t.Helper()
	titles := []string{}
	if shell {
		titles = append(titles, "~/carl-github/term-fleet")
	}
	w, _ := newTestShellWindow(t, titles...)
	w.fleet = newTestFleet(t)
	if _, err := w.store.Update(func(s *settings.Settings) { s.Theme = mode }); err != nil {
		t.Fatal(err)
	}
	if !shell {
		return w
	}
	w.tabs[1].term = fedTerminal(t, previewScreen())
	return w
}

// previewFleet gives the preview window a fleet: the relay's roster, with one
// other machine online and a shell running on it, as the panel draws them.
func previewFleet(w *window) {
	w.roster = []tunnel.Machine{{
		ID:     "mac-studio",
		Name:   "mac-studio",
		Host:   protocol.Host{Hostname: "mac-studio.local", Platform: "darwin", Arch: "arm64"},
		Online: true,
	}}
	w.remoteSessions = map[string][]protocol.Summary{
		"mac-studio": {
			{
				SessionID: "s7",
				Title:     "~/work/term-fleet",
				Shell:     "/bin/zsh",
				Cols:      120,
				Rows:      40,
				Alive:     true,
			},
			{
				SessionID: "s8",
				Title:     "~/work/api",
				Shell:     "/bin/zsh",
				Cols:      120,
				Rows:      40,
				Alive:     true,
			},
		},
	}
}

// previewScreen is what the pane in the pictures is showing: a session's worth
// of a person's day, so the preview has something to render.
func previewScreen() string {
	return "\x1b[38;2;164;75;240m~/carl-github/term-fleet\x1b[0m \x1b[38;2;138;147;168mon \x1b[0m\x1b[38;2;62;216;121mfeat/desktop-shell\x1b[0m\r\n" +
		"\x1b[38;2;138;147;168m$\x1b[0m git status --short\r\n" +
		" \x1b[38;2;240;168;60mM\x1b[0m cmd/tfdesktop/app.go\r\n" +
		" \x1b[38;2;240;168;60mM\x1b[0m cmd/tfdesktop/nav.go\r\n" +
		"\x1b[38;2;62;216;121m??\x1b[0m cmd/tfdesktop/palette.go\r\n" +
		"\x1b[38;2;62;216;121m??\x1b[0m cmd/tfdesktop/shell.go\r\n" +
		"\x1b[38;2;138;147;168m$\x1b[0m go test ./...\r\n" +
		"ok  \tgithub.com/CarlOpenLab/term-fleet/cmd/tfdesktop\t0.6s\r\n" +
		"ok  \tgithub.com/CarlOpenLab/term-fleet/internal/settings\t0.1s\r\n" +
		"\x1b[38;2;138;147;168m$\x1b[0m "
}

// nopConn is a terminal's connection to nothing: reads block, so the shell it
// drives is a terminal that is simply quiet, and writes go nowhere.
type nopConn struct{ reader *io.PipeReader }

func (c nopConn) Read(p []byte) (int, error)  { return c.reader.Read(p) }
func (c nopConn) Write(p []byte) (int, error) { return len(p), nil }
func (c nopConn) Close() error                { return c.reader.Close() }

// fedTerminal is a terminal with no program behind it, holding one screenful of
// text, which is all a picture needs.
func fedTerminal(t *testing.T, screen string) *terminal.Terminal {
	t.Helper()
	reader, _ := io.Pipe()
	term, err := terminal.New(terminal.Options{
		Conn:      nopConn{reader: reader},
		Theme:     terminalTheme(false),
		DarkTheme: terminalTheme(true),
	})
	if err != nil {
		t.Fatalf("open a terminal for the preview: %v", err)
	}
	term.Resize(previewCols, previewRows)
	term.Feed([]byte(screen))
	return term
}
