package main

import (
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/settings"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
	"github.com/CarlOpenLab/term-fleet/locales"
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
		// remoteTaken is that shell, driven by the machine it runs on: the state
		// the light along the pane's edge is deliberately not drawn for.
		remoteTaken bool
		// ended is a shell that has exited: its tab stays so its last output can
		// be read, and the way out of that state is on the tab, in the header and
		// on the row.
		ended bool
		// split is the body with two panes: "side" for beside each other (⌘D)
		// and "stack" for one above the other (⌘⇧D), each with a shell of its
		// own and the divider the pointer drags between them.
		split string
	}{
		{"shell-dark", themeDark, true, false, false, false, false, false, false, false, ""},
		{"shell-light", themeLight, true, false, false, false, false, false, false, false, ""},
		{"empty-dark", themeDark, false, false, false, false, false, false, false, false, ""},
		{"empty-light", themeLight, false, false, false, false, false, false, false, false, ""},
		// A shell of this machine's, another client driving it: the light along
		// the pane's edge, the panel under it, and the mark on its row.
		{"taken-dark", themeDark, true, true, false, false, false, false, false, false, ""},
		{"taken-light", themeLight, true, true, false, false, false, false, false, false, ""},
		// A shell that has ended: 已退出 on the pane, on the tab and on its row,
		// with 关闭 where a running shell's row has 结束.
		{"ended-dark", themeDark, true, false, false, false, false, false, false, true, ""},
		{"ended-light", themeLight, true, false, false, false, false, false, false, true, ""},
		// A fleet: the 机器 panel as a list you switch between, with this machine
		// and one other.
		{"fleet-dark", themeDark, true, false, true, false, false, false, false, false, ""},
		{"fleet-light", themeLight, true, false, true, false, false, false, false, false, ""},
		// Another machine picked: its row carries the selection, and the 会话 panel
		// below shows the shells that machine is running rather than this one's.
		{"fleet-picked-dark", themeDark, true, false, true, true, false, false, false, false, ""},
		{"fleet-picked-light", themeLight, true, false, true, true, false, false, false, false, ""},
		// Picking an unreachable machine: the dialog that asks the relay for a
		// short-lived credential first.
		{"fleet-ask-dark", themeDark, true, false, true, false, true, false, false, false, ""},
		{"fleet-ask-light", themeLight, true, false, true, false, true, false, false, false, ""},
		// The other machine's shell on screen: its row carries the accent bar the
		// picked machine carries, and this machine's shell is a tab behind it.
		{"fleet-remote-dark", themeDark, true, false, true, true, false, true, false, false, ""},
		{"fleet-remote-light", themeLight, true, false, true, true, false, true, false, false, ""},
		// That same shell driven by the machine it runs on: 只读 on the pane and on
		// the row, and no light. A pane that lit up for a process on a machine the
		// person here is not sitting at would be spending the light on a fact they
		// have no stake in.
		{"fleet-taken-remote-dark", themeDark, true, false, true, true, false, true, true, false, ""},
		{"fleet-taken-remote-light", themeLight, true, false, true, true, false, true, true, false, ""},
		// The body split in two, both ways the key makes one: two shells, each on
		// its own sheet, with the divider between them.
		{"split-side-dark", themeDark, true, false, false, false, false, false, false, false, "side"},
		{"split-side-light", themeLight, true, false, false, false, false, false, false, false, "side"},
		{"split-stack-dark", themeDark, true, false, false, false, false, false, false, false, "stack"},
		{"split-stack-light", themeLight, true, false, false, false, false, false, false, false, "stack"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := previewWindow(t, tc.mode, tc.shell)
			if tc.taken {
				w.tabs[1].setLease(protocol.RoleViewer, "c9", 4)
				w.leaseOpen = true
			}
			if tc.ended {
				// A shell that exited on its own, through the same door the exit
				// frame uses: the tab is kept to read, and every control left on
				// screen is the way out of it.
				w.tabs[1].setLease(protocol.RoleViewer, "", 0)
				w.tabs[1].finish(0)
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
				if tc.remoteTaken {
					remote.setLease(protocol.RoleViewer, "mac-studio", 1)
				}
				w.selectTab(remote)
			}
			if tc.split != "" {
				// The body as ⌘D or ⌘⇧D leaves it: the shell that was on screen
				// beside a fresh one, each with its own sheet, the fresh pane the
				// one on screen.
				fresh := &tab{
					id: "s3", machine: machineLocal, title: "~/work/api",
					ordinal: 2, term: fedTerminal(t, previewScreen()), alive: true,
				}
				fresh.setLease(protocol.RoleController, "", 2)
				w.tabs[2] = fresh
				w.order = append(w.order, 2)
				w.split = &splitNode{
					side: tc.split == "side", pct: splitHalf,
					a: &splitNode{tab: w.tabs[1]}, b: &splitNode{tab: fresh},
				}
				w.focus = w.split.b
				w.selectTabRaw(fresh)
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
		{"settings-dark-terminal", themeDark, paneTerminal, locales.SettingsPaneTerminal()},
		{"settings-dark-relay", themeDark, paneRelay, locales.SettingsPaneRelay()},
		{"settings-dark-server", themeDark, paneServer, locales.SettingsPaneServer()},
		{"settings-dark-updates", themeDark, paneUpdates, locales.SettingsPaneUpdates()},
		{"settings-light-terminal", themeLight, paneTerminal, locales.SettingsPaneTerminal()},
		{"settings-light-relay", themeLight, paneRelay, locales.SettingsPaneRelay()},
		{"settings-light-server", themeLight, paneServer, locales.SettingsPaneServer()},
		{"settings-light-updates", themeLight, paneUpdates, locales.SettingsPaneUpdates()},
		{"settings-dark-appearance", themeDark, paneAppearance, locales.SettingsPaneAppearance()},
		{"settings-light-appearance", themeLight, paneAppearance, locales.SettingsPaneAppearance()},
		{"settings-light-notifications", themeLight, paneNotifications, locales.SettingsPaneNotifications()},
		{"settings-light-keyboard", themeLight, paneKeyboard, locales.SettingsPaneKeyboard()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := settings.Open(t.TempDir())
			if _, err := store.Update(func(s *settings.Settings) { s.Theme = tc.mode }); err != nil {
				t.Fatal(err)
			}
			sw := newSettingsWindow(&window{}, store, newTestFleet(t))
			sw.pane = tc.pane
			tt := ui.NewTester(sw.view, 720, 560)
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
	w.frameStub = &fakeWindowFrame{}
	if _, err := w.store.Update(func(s *settings.Settings) { s.Theme = mode }); err != nil {
		t.Fatal(err)
	}
	if !shell {
		return w
	}
	// The shell in the picture is a live one this window is driving: a preview
	// drawn from a shell that has ended, or from one nobody holds the lease of,
	// would show the locked-down composer and the ended marks on every screen,
	// which are the two states the window is least often in.
	w.tabs[1].id = "s2"
	w.tabs[1].alive = true
	w.tabs[1].setLease(protocol.RoleController, "", 1)
	w.tabs[1].term = fedTerminal(t, previewScreen())

	// The rows the picture is about: one working with a progress it reported, one
	// waiting for the user with a permission prompt, and one that finished while
	// nobody was looking. They are what the state glyphs, the message chips and
	// the foot's counts are drawn from.
	now := time.Now().UnixMilli()
	w.tabs[2] = &tab{id: "s9", machine: machineLocal, title: "review -- fix login", ordinal: 2, alive: true, term: fedTerminal(t, previewScreen())}
	w.tabs[3] = &tab{id: "s10", machine: machineLocal, title: "docs dev server", ordinal: 3, alive: true, term: fedTerminal(t, previewScreen())}
	w.tabs[2].setLease(protocol.RoleViewer, "c4", 2)
	w.tabs[3].setLease(protocol.RoleViewer, "", 0)
	w.order = append(w.order, 2, 3)
	w.live = []protocol.Summary{
		{SessionID: "s2", Title: "~/carl-github/term-fleet", Alive: true, Status: "working", Progress: 40, Message: "running the tests", Process: "claude", Since: now - 42_000},
		{SessionID: "s9", Title: "review -- fix login", Alive: true, Status: "waiting", Unread: true, Kind: "permission", Message: "Approve the production deploy?", Since: now - 2*60_000},
		{SessionID: "s10", Title: "docs dev server", Alive: true, Status: "idle", Unread: true, Since: now - 3*3_600_000},
	}
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
				Status:    "working",
				Progress:  12,
				Message:   "npm run dev",
				Since:     time.Now().Add(-8 * time.Second).UnixMilli(),
			},
			{
				SessionID: "s8",
				Title:     "~/work/api",
				Shell:     "/bin/zsh",
				Cols:      120,
				Rows:      40,
				Alive:     true,
				Status:    "waiting",
				Unread:    true,
				Kind:      "question",
				Message:   "Which database should I migrate?",
				Since:     time.Now().Add(-90 * time.Second).UnixMilli(),
			},
		},
	}
}

// previewScreen is what the pane in the pictures is showing: a session's worth
// of a person's day, so the preview has something to render. The colours are the
// terminal palette in palette.go — the violet the chrome is drawn in, the green
// of a pass, the amber of a modified file — so the picture shows the sheet a
// real shell gets rather than a second, prettier one.
func previewScreen() string {
	return "\x1b[38;2;182;166;255m~/carl-github/term-fleet\x1b[0m \x1b[38;2;139;144;162mon \x1b[0m\x1b[38;2;111;211;154mfeat/desktop-shell\x1b[0m\r\n" +
		"\x1b[38;2;139;144;162m$\x1b[0m git status --short\r\n" +
		" \x1b[38;2;230;192;125mM\x1b[0m cmd/tfdesktop/app.go\r\n" +
		" \x1b[38;2;230;192;125mM\x1b[0m cmd/tfdesktop/nav.go\r\n" +
		"\x1b[38;2;111;211;154m??\x1b[0m cmd/tfdesktop/palette.go\r\n" +
		"\x1b[38;2;111;211;154m??\x1b[0m cmd/tfdesktop/shell.go\r\n" +
		"\x1b[38;2;139;144;162m$\x1b[0m go test ./...\r\n" +
		"ok  \tgithub.com/CarlOpenLab/term-fleet/cmd/tfdesktop\t0.6s\r\n" +
		"ok  \tgithub.com/CarlOpenLab/term-fleet/internal/settings\t0.1s\r\n" +
		"\x1b[38;2;139;144;162m$\x1b[0m "
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
