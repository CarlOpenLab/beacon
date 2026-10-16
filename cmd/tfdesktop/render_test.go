package main

import (
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/settings"
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
	}{
		{"shell-dark", themeDark, true},
		{"shell-light", themeLight, true},
		{"empty-dark", themeDark, false},
		{"empty-light", themeLight, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := previewWindow(t, tc.mode, tc.shell)
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

// previewWindow is the window the previews draw: the appearance asked for, and,
// when a shell is wanted, one shell with a screenful of text in it.
func previewWindow(t *testing.T, mode string, shell bool) *window {
	t.Helper()
	titles := []string{}
	if shell {
		titles = append(titles, "~/carl-github/term-fleet")
	}
	w, _ := newTestShellWindow(t, titles...)
	if _, err := w.store.Update(func(s *settings.Settings) { s.Theme = mode }); err != nil {
		t.Fatal(err)
	}
	if !shell {
		return w
	}
	w.tabs["s1"].term = fedTerminal(t, previewScreen())
	return w
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
