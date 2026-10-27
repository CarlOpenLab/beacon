package main

// The clipboard belongs to the machine the keyboard is on, not to the machine
// the shell runs on. Ctrl+V at a remote shell used to be sent across as ^V,
// which programs that paste images (Claude Code among them) read as "read the
// clipboard" — and the clipboard they read is the one at the far end. The keys
// are bound on this side for a remote tab, and left alone for a local shell.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/protocol"
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

// Exercise the actual file clipboard hook, local handshake and hub upload, then
// verify the path delivered to the terminal's input stream names those bytes.
func TestLocalFilePasteUploadsAndSendsPath(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	w.local.onSessions = w.sessionsArrived
	t.Cleanup(func() { w.local.close("test done") })
	tb := w.openTab(machineLocal, "")
	w.local.create(80, 24, "", "", "")
	waitForPaste(t, "session control and file capability", func() bool {
		w.drain()
		return tb.held() && w.local.capable(protocol.CapFile)
	})

	// Capture the final terminal input while all upload frames still travel
	// through the real local pipe and hub.
	inputs := make(chan string, 1)
	w.local.sendMu.Lock()
	send := w.local.send
	w.local.send = func(m protocol.Msg) bool {
		if m.T == protocol.CTInput {
			inputs <- string(m.Data)
			return true
		}
		return send(m)
	}
	w.local.sendMu.Unlock()
	payload := []byte("clipboard image payload")
	source := filepath.Join(t.TempDir(), "screenshot.png")
	if err := os.WriteFile(source, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := transfer.FileData(source)
	if err != nil {
		t.Fatal(err)
	}
	if !w.pasteInto(tb, data) {
		t.Fatal("file paste was not handled")
	}
	var path string
	waitForPaste(t, "uploaded path to reach terminal input", func() bool {
		w.drain()
		select {
		case text := <-inputs:
			path = strings.TrimSuffix(strings.TrimPrefix(text, "'"), "' ")
			return true
		default:
			return false
		}
	})
	t.Cleanup(func() { _ = os.Remove(path) })
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("pasted path %q: bytes=%q err=%v", path, got, err)
	}
}

func TestLocalImagePasteForwardsAgentGesture(t *testing.T) {
	reader, _ := io.Pipe()
	conn := &pasteConn{reader: reader}
	w := &window{}
	tb := &tab{machine: machineLocal, id: "s1", role: protocol.RoleController,
		sess: newSession("s1", newLink(), nil)}
	term, err := terminal.New(terminal.Options{
		Conn:     conn,
		Keybinds: []terminal.Keybind{{Mods: ui.Cmd, Key: ui.KeyV, Action: terminal.ActionPaste}},
		OnPaste:  func(d transfer.Data) bool { return w.pasteInto(tb, d) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tb.term = term
	tt := ui.NewTester(func(c *ui.Context) {
		terminal.View(c, term).Fill().AutoFocus()
	}, 500, 240)
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, []byte("image")))))
	tt.Key(ui.Cmd, ui.KeyV)
	waitForPaste(t, "one raw Ctrl+V to reach the local agent", func() bool {
		return conn.text() == "\x16"
	})
	// No upload capability is required for the local image gesture, and text
	// alongside an image must still follow the ordinary terminal paste path.
	tt.SetClipboardData(transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.PNG, []byte("image")),
		transfer.Bytes(transfer.Text, []byte("some text")),
	)))
	tt.Key(ui.Cmd, ui.KeyV)
	waitForPaste(t, "text paste without another image gesture", func() bool {
		return conn.text() == "\x16some text"
	})
	tb.role = protocol.RoleViewer
	if !w.pasteInto(tb, transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, []byte("image"))))) {
		t.Fatal("read-only image paste should be consumed")
	}
	if got := conn.text(); got != "\x16some text" {
		t.Fatalf("read-only paste sent input: %q", got)
	}
}

func TestRemoteImagePasteStillUploadsInsteadOfForwardingGesture(t *testing.T) {
	reader, _ := io.Pipe()
	conn := &pasteConn{reader: reader}
	term, err := terminal.New(terminal.Options{Conn: conn})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	l := newLink()
	l.setHost(nil, []string{protocol.CapFile})
	var uploaded bytes.Buffer
	l.setSend(func(m protocol.Msg) bool {
		switch m.T {
		case protocol.CTFileChunk:
			uploaded.Write(m.Data)
		case protocol.CTFileDone:
			l.deliverFile(protocol.Msg{T: protocol.STFileSaved, ReqID: m.ReqID, Path: "/tmp/remote-shot.png"})
		}
		return true
	})
	w := &window{}
	tb := &tab{machine: "remote", id: "s1", role: protocol.RoleController,
		sess: newSession("s1", l, nil), term: term}
	payload := []byte("remote image bytes")
	if !w.pasteInto(tb, transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, payload)))) {
		t.Fatal("remote image was not handled")
	}
	waitForPaste(t, "remote image path", func() bool {
		w.drain()
		return conn.text() == "'/tmp/remote-shot.png' "
	})
	if !bytes.Equal(uploaded.Bytes(), payload) {
		t.Fatalf("uploaded %q, want %q", uploaded.Bytes(), payload)
	}
}

func TestRemoteHelloAnnouncesFileUploadCapability(t *testing.T) {
	l := &relayLink{link: newLink(), machine: "remote"}
	var sent []protocol.Msg
	l.setSend(func(m protocol.Msg) bool {
		sent = append(sent, m)
		return true
	})
	(relayHandlers{l: l}).Open()
	if len(sent) == 0 || sent[0].T != protocol.CTHello || !protocol.HasCap(sent[0].Caps, protocol.CapFile) {
		t.Fatalf("remote handshake did not announce upload support: %+v", sent)
	}
}

// Windows cannot carry a terminal's paste event to a program: its console host
// reads what is written to it as keyboard input, so an OSC 5522 offer is
// dropped rather than handed to the program. A program that asks for paste
// events — omp and other agents do, for images — would then lose a text paste
// while images, which this app handles itself, still arrived. The text is
// pasted as text.
func TestTextPasteReachesAProgramThatAsksForPasteEvents(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only ConPTY drops a paste event")
	}
	reader, _ := io.Pipe()
	conn := &pasteConn{reader: reader}
	term, err := terminal.New(terminal.Options{Conn: conn})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	// What the program asked for: paste events, and bracketed pastes.
	term.Feed([]byte("$ \x1b[?5522h\x1b[?2004h"))
	w := &window{}
	tb := &tab{machine: machineLocal, id: "s1", role: protocol.RoleController,
		sess: newSession("s1", newLink(), nil), term: term}
	if !w.pasteInto(tb, transfer.TextData("粘贴 text")) {
		t.Fatal("text paste was not taken")
	}
	waitForPaste(t, "the text as a bracketed paste", func() bool {
		return conn.text() == "\x1b[200~粘贴 text\x1b[201~"
	})
}
