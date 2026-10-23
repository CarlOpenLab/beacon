package main

// The clipboard belongs to the machine the keyboard is on, not to the machine
// the shell runs on. Ctrl+V at a remote shell used to be sent across as ^V,
// which programs that paste images (Claude Code among them) read as "read the
// clipboard" — and the clipboard they read is the one at the far end. The keys
// are bound on this side for a remote tab, and left alone for a local shell.

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// pasteConn records what the terminal sends and blocks on reads, so the test
// has a shell with nothing behind it.
type pasteConn struct {
	reader *io.PipeReader

	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *pasteConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
func (c *pasteConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.buf.Write(p)
	c.mu.Unlock()
	return len(p), nil
}
func (c *pasteConn) Close() error { return c.reader.Close() }

// waitForPaste polls for a condition for two seconds. The test package's usual
// waitFor lives in a !windows file, and this test runs on every platform.
func waitForPaste(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s: timed out", what)
}

func (c *pasteConn) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// Ctrl+V pastes this machine's clipboard into a remote shell.
func TestCtrlVPastesTheLocalClipboardIntoARemoteShell(t *testing.T) {
	reader, _ := io.Pipe()
	conn := &pasteConn{reader: reader}
	term, err := terminal.New(terminal.Options{
		Conn:     conn,
		Keybinds: terminalKeybinds("win-2024"),
	})
	if err != nil {
		t.Fatalf("open a terminal: %v", err)
	}
	defer term.Close()
	term.Feed([]byte("$ "))

	tt := ui.NewTester(func(c *ui.Context) {
		terminal.View(c, term).Fill().AutoFocus()
	}, 500, 240)
	tt.SetClipboard("本地剪贴板")
	tt.Key(ui.Ctrl, ui.KeyV)

	waitForPaste(t, "the local clipboard to reach the remote shell", func() bool {
		return conn.text() == "本地剪贴板"
	})

	// With nothing to paste the key is still the program's: a performable
	// binding falls through, so Control-V keeps meaning quoted-insert.
	tt.SetClipboard("")
	tt.Key(ui.Ctrl, ui.KeyV)
	waitForPaste(t, "the empty clipboard to fall through as ^V", func() bool {
		return conn.text() == "本地剪贴板\x16"
	})
}

// A local shell's Ctrl+V is untouched: it is quoted-insert, not paste.
func TestCtrlVOnALocalShellIsNotRebound(t *testing.T) {
	if binds := terminalKeybinds(machineLocal); binds != nil {
		t.Fatalf("a local shell was given remote keybinds: %+v", binds)
	}
}

// And a remote shell gets both spellings, so a Mac keyboard and a Windows one
// paste the same clipboard.
func TestARemoteShellBindsBothPasteSpellings(t *testing.T) {
	binds := terminalKeybinds("win-2024")
	want := []terminal.Keybind{
		{Mods: ui.Ctrl, Key: ui.KeyV, Action: terminal.ActionPaste, Performable: true},
		{Mods: ui.Ctrl | ui.Shift, Key: ui.KeyV, Action: terminal.ActionPaste, Performable: true},
	}
	if len(binds) != len(want) {
		t.Fatalf("remote keybinds = %+v, want %+v", binds, want)
	}
	for i := range want {
		if binds[i] != want[i] {
			t.Errorf("binding %d = %+v, want %+v", i, binds[i], want[i])
		}
	}
}

// A pasted file's path reaches the shell as one word, in the quoting the shell
// on that machine reads: a screenshot into a directory with a space in it must
// not become two arguments.
func TestShellQuote(t *testing.T) {
	cases := []struct {
		path     string
		platform string
		want     string
	}{
		{"/tmp/beacon-paste/shot.png", "darwin", "'/tmp/beacon-paste/shot.png'"},
		{"/tmp/it's here.png", "linux", `'/tmp/it'\''s here.png'`},
		{`C:\Users\me\shot.png`, "windows", `'C:\Users\me\shot.png'`},
		{`C:\it's.png`, "windows", `'C:\it''s.png'`},
	}
	for _, tc := range cases {
		if got := shellQuote(tc.path, tc.platform); got != tc.want {
			t.Errorf("shellQuote(%q, %q) = %q, want %q", tc.path, tc.platform, got, tc.want)
		}
	}
}
