//go:build !windows

package main

// The reported failure: a machine whose shell is busy and held by that
// machine's own window (the Mac running an AI session), viewed from another
// machine through the relay. The viewer attaches and must be caught up by the
// ring replay — the tab renders the shell's screen, not a blank pane.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/agent"
	"github.com/CarlOpenLab/term-fleet/internal/daemon"
	"github.com/CarlOpenLab/term-fleet/internal/hub"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/relay"
	"github.com/CarlOpenLab/term-fleet/internal/settings"
)

func TestViewerSeesBusyHeldRemoteShell(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// The other machine: its daemon, its hub, and its own window's link.
	other, err := daemon.New(daemon.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("daemon: %v", err)
	}
	go func() { _ = other.Serve(ctx) }()
	t.Cleanup(func() { _ = other.Close() })
	ownerPipe := hub.NewPipe()
	defer ownerPipe.Close()
	go other.Hub().ServeLocal(ownerPipe.Local())
	owner := newPipeClient(t, ownerPipe)

	rl, err := relay.New(relay.Config{
		Devices:     map[string]string{"member-2": "membersec", "desk-1": "desksec"},
		ClientToken: "fleettoken",
	})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", rl.ServeAgent)
	mux.HandleFunc("/relay/client", rl.ServeClient)
	mux.HandleFunc("/relay/machines", rl.Machines)
	mux.HandleFunc("/relay/control-token", rl.ControlToken)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	ag, err := agent.New(agent.Config{
		URL:         "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/agent",
		DeviceID:    "member-2",
		Secret:      "membersec",
		MachineName: "other box",
		Hub:         other.Hub(),
		MinBackoff:  10 * time.Millisecond,
		MaxBackoff:  50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("agent: %v", err)
	}
	go func() { _ = ag.Run(ctx) }()

	// This machine's window, reaching the fleet the way the app does.
	f := newTestFleet(t)
	store := settings.Open(t.TempDir())
	relayURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/agent"
	if err := f.SetRelay(relayURL, "desk-1", "desksec", "", true); err != nil {
		t.Fatalf("SetRelay: %v", err)
	}
	if err := f.Reach(); err != nil {
		t.Fatalf("Reach: %v", err)
	}
	w := newWindow(f, store, "", "")
	waitFor(t, "the roster", 20*time.Second, func() bool {
		w.drain()
		for _, m := range w.roster {
			if m.ID == "member-2" && m.Online {
				return true
			}
		}
		return false
	})

	// The other machine's window opens a shell and holds it: the machine's own
	// shell, held by the machine's own client — the AI session on the Mac.
	owner.send(protocol.Msg{T: protocol.CTCreate, Cols: 80, Rows: 24, Shell: "/bin/sh"})
	created := owner.waitFor("created", func(m protocol.Msg) bool { return m.T == protocol.STCreated })
	shell := created.SessionID
	owner.waitFor("the owner holds the lease", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})

	// The shell prints a banner and then keeps producing output without pause,
	// the way a session running an AI does.
	owner.send(protocol.Msg{T: protocol.CTInput, SessionID: shell,
		Data: []byte("printf 'BUSY_BANNER\\n'; while true; do printf 'TICK\\n'; sleep 0.02; done\n")})
	waitFor(t, "the banner in the ring", 10*time.Second, func() bool {
		s, ok := other.Hub().Session(shell)
		if !ok {
			return false
		}
		data, _, _ := s.Ring().ReadFrom(0)
		return strings.Contains(string(data), "BUSY_BANNER")
	})

	// This window looks at the machine and opens a tab on the busy shell. The
	// owner holds the lease, so the tab arrives as a viewer.
	w.lookAtMachine("member-2")
	waitFor(t, "the other machine's session list", 20*time.Second, func() bool {
		w.drain()
		for _, s := range w.remoteSessions["member-2"] {
			if s.SessionID == shell {
				return true
			}
		}
		return false
	})
	tb := w.openTab("member-2", shell)
	if tb == nil {
		t.Fatal("no tab for the busy remote shell")
	}
	if tb.held() {
		t.Error("the viewer was handed the lease of a shell the machine's own window holds")
	}

	// The tab renders the shell's screen: the replayed banner, and the ticks
	// that keep arriving after the attach.
	waitFor(t, "the viewer's terminal to render the replay", 20*time.Second, func() bool {
		w.drain()
		return strings.Contains(tb.term.Text(), "BUSY_BANNER")
	})
	waitFor(t, "the viewer's terminal to render live output", 20*time.Second, func() bool {
		w.drain()
		return strings.Count(tb.term.Text(), "TICK") > 3
	})
}

// The pushed-over screen: a shell on another machine is 120 columns wide, and a
// window watching it from here drew the same bytes at the width of its own pane
// — the program's hundred-column line wrapped where the machine's window shows
// one line, and its next redraw, which moves the cursor from where it thinks it
// is, landed a row off. The pane is a window onto the shell's screen, so it lays
// that screen out at the size the shell has, and shows the same screen the
// machine's own window does.
func TestViewerLaysTheScreenOutAtTheShellsSize(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// The other machine: its daemon, its hub, and its own window's link.
	other, err := daemon.New(daemon.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("daemon: %v", err)
	}
	go func() { _ = other.Serve(ctx) }()
	t.Cleanup(func() { _ = other.Close() })
	ownerPipe := hub.NewPipe()
	defer ownerPipe.Close()
	go other.Hub().ServeLocal(ownerPipe.Local())
	owner := newPipeClient(t, ownerPipe)

	rl, err := relay.New(relay.Config{
		Devices:     map[string]string{"member-2": "membersec", "desk-1": "desksec"},
		ClientToken: "fleettoken",
	})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", rl.ServeAgent)
	mux.HandleFunc("/relay/client", rl.ServeClient)
	mux.HandleFunc("/relay/machines", rl.Machines)
	mux.HandleFunc("/relay/control-token", rl.ControlToken)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	ag, err := agent.New(agent.Config{
		URL:         "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/agent",
		DeviceID:    "member-2",
		Secret:      "membersec",
		MachineName: "other box",
		Hub:         other.Hub(),
		MinBackoff:  10 * time.Millisecond,
		MaxBackoff:  50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("agent: %v", err)
	}
	go func() { _ = ag.Run(ctx) }()

	f := newTestFleet(t)
	store := settings.Open(t.TempDir())
	relayURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/agent"
	if err := f.SetRelay(relayURL, "desk-1", "desksec", "", true); err != nil {
		t.Fatalf("SetRelay: %v", err)
	}
	if err := f.Reach(); err != nil {
		t.Fatalf("Reach: %v", err)
	}
	w := newWindow(f, store, "", "")
	waitFor(t, "the roster", 20*time.Second, func() bool {
		w.drain()
		for _, m := range w.roster {
			if m.ID == "member-2" && m.Online {
				return true
			}
		}
		return false
	})

	// The other machine's window opens a big shell — 120 by 30 — and holds it.
	owner.send(protocol.Msg{T: protocol.CTCreate, Cols: 120, Rows: 30, Shell: "/bin/sh"})
	created := owner.waitFor("created", func(m protocol.Msg) bool { return m.T == protocol.STCreated })
	shell := created.SessionID
	owner.waitFor("the owner holds the lease", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})

	// A hundred columns of one line: within the shell's 120 columns, and past
	// the 80 this window's terminal is until a frame gives it a size.
	owner.send(protocol.Msg{T: protocol.CTInput, SessionID: shell,
		Data: []byte("printf 'A%.0s' $(seq 1 100); printf 'TAIL\\n'\n")})
	waitFor(t, "the line in the ring", 20*time.Second, func() bool {
		s, ok := other.Hub().Session(shell)
		if !ok {
			return false
		}
		data, _, _ := s.Ring().ReadFrom(0)
		return strings.Contains(string(data), "TAIL")
	})

	w.lookAtMachine("member-2")
	waitFor(t, "the other machine's session list", 20*time.Second, func() bool {
		w.drain()
		for _, s := range w.remoteSessions["member-2"] {
			if s.SessionID == shell {
				return true
			}
		}
		return false
	})
	tb := w.openTab("member-2", shell)
	if tb == nil {
		t.Fatal("no tab for the remote shell")
	}
	if tb.held() {
		t.Fatal("the viewer was handed the lease of a shell the machine's own window holds")
	}

	// The pane is a window onto the shell's screen, so the screen is the shell's
	// size rather than the pane's.
	waitFor(t, "the viewer's screen to take the shell's size", 20*time.Second, func() bool {
		w.drain()
		cols, rows := tb.term.Size()
		return cols == 120 && rows == 30
	})

	// And the line the program wrote for that width is one line here too: laid
	// out at 80 columns it would be 80 A's, a wrapped row, and the other 20.
	waitFor(t, "the whole line, unwrapped", 20*time.Second, func() bool {
		w.drain()
		return strings.Contains(tb.term.Text(), strings.Repeat("A", 100)+"TAIL")
	})

	// A pane narrower than the screen is a window onto it, not a new screen: a
	// frame laid out here must not reflow what the shell wrote for 120 columns.
	tt := ui.NewTester(func(c *ui.Context) { terminal.View(c, tb.term).Fill() }, 400, 200)
	for i := 0; i < 3; i++ {
		tt.Frame()
		w.drain()
	}
	// The view hands the conn its size off the frame, on its own goroutine.
	time.Sleep(200 * time.Millisecond)
	if cols, rows := tb.term.Size(); cols != 120 || rows != 30 {
		t.Fatalf("the pane reflowed the screen to %dx%d, want the shell's 120x30", cols, rows)
	}
	w.drain()
	if tb.held() {
		t.Fatal("a frame of a viewer's pane took the lease")
	}
	if !strings.Contains(tb.term.Text(), strings.Repeat("A", 100)+"TAIL") {
		t.Fatal("the pane wrapped the line the shell's own screen has whole")
	}
}

// The takeover and the hand-back: a shell this window takes from the machine's
// own window becomes this pane's size, and when it is released the machine's
// window takes it back at its own — which the pane follows. A viewer that kept
// drawing the shell at the width it had while it drove it is the garbled screen
// a person sees right after 释放: every line wrapped where the machine's window
// no longer wraps it, and the program's next redraw a row off.
func TestAReleasedShellComesBackAtTheOwnersSize(t *testing.T) {
	f := newTestFleet(t)
	w, _ := newTestShellWindow(t)
	w.fleet = f
	w.local = openLocalLink(f.Hub())
	w.local.onSessions = w.sessionsArrived
	w.local.onGone = w.linkGone

	// The machine's own window: a shell at 120 by 30, and it holds it.
	ownerPipe := hub.NewPipe()
	defer ownerPipe.Close()
	go f.Hub().ServeLocal(ownerPipe.Local())
	owner := newPipeClient(t, ownerPipe)
	owner.send(protocol.Msg{T: protocol.CTCreate, Cols: 120, Rows: 30, Shell: "/bin/sh"})
	created := owner.waitFor("created", func(m protocol.Msg) bool { return m.T == protocol.STCreated })
	shell := created.SessionID
	owner.waitFor("the owner holds the lease", func(m protocol.Msg) bool {
		return m.T == protocol.STRole && m.Role == protocol.RoleController
	})

	// This window watches it, and the pane shows the shell's own screen.
	tb := w.openTab(machineLocal, shell)
	if tb == nil {
		t.Fatal("no tab for the shell")
	}
	waitFor(t, "the viewer's screen to take the shell's size", 20*time.Second, func() bool {
		w.drain()
		cols, rows := tb.term.Size()
		return cols == 120 && rows == 30
	})
	if tb.held() {
		t.Fatal("the viewer was handed the lease of a shell another window holds")
	}

	// A pane of this window's own, narrower than the screen it shows: watching
	// does not reflow the shell.
	tt := ui.NewTester(func(c *ui.Context) { terminal.View(c, tb.term).Fill() }, 420, 200)
	for i := 0; i < 3; i++ {
		tt.Frame()
		w.drain()
	}
	time.Sleep(200 * time.Millisecond)
	if cols, rows := tb.term.Size(); cols != 120 || rows != 30 {
		t.Fatalf("the watching pane reflowed the screen to %dx%d, want 120x30", cols, rows)
	}
	viewCols, viewRows := tb.conn.viewSize()
	if viewCols <= 0 || viewRows <= 0 {
		t.Fatalf("the view never told the conn its size: %dx%d", viewCols, viewRows)
	}

	// Taken: the shell is this pane's size from here, and the hub is told it.
	tb.conn.claiming(true)
	waitFor(t, "this window to hold the lease", 20*time.Second, func() bool {
		w.drain()
		return tb.held()
	})
	waitFor(t, "the screen to take the pane's size", 20*time.Second, func() bool {
		w.drain()
		cols, rows := tb.term.Size()
		return cols == viewCols && rows == viewRows
	})
	waitFor(t, "the hub to report the pane's size", 20*time.Second, func() bool {
		s, ok := f.Hub().Session(shell)
		if !ok {
			return false
		}
		sum := s.Summary()
		return sum.Cols == viewCols && sum.Rows == viewRows
	})

	// Released: the machine's window claims its shell back and gives it its own
	// size again, which is the size this pane must lay the shell out at.
	tb.conn.releasing()
	owner.send(protocol.Msg{T: protocol.CTClaim, SessionID: shell, Steal: true})
	owner.send(protocol.Msg{T: protocol.CTResize, SessionID: shell, Cols: 120, Rows: 30})
	waitFor(t, "the pane to follow the shell back to its own size", 20*time.Second, func() bool {
		w.drain()
		cols, rows := tb.term.Size()
		return cols == 120 && rows == 30
	})
	if tb.held() {
		t.Fatal("the released shell stayed this window's")
	}

	// And the output the shell writes at that size lands unwrapped, which is the
	// whole point: the same screen the machine's window shows.
	owner.send(protocol.Msg{T: protocol.CTInput, SessionID: shell,
		Data: []byte("printf 'B%.0s' $(seq 1 100); printf 'BACK\\n'\n")})
	waitFor(t, "the line written after the hand-back", 20*time.Second, func() bool {
		w.drain()
		return strings.Contains(tb.term.Text(), strings.Repeat("B", 100)+"BACK")
	})

	// Taken once more: the shell is this pane's size again, even though this
	// conn told the hub that size before it gave the shell away — a window that
	// stayed quiet on that reasoning would leave the shell at the machine's width
	// with this pane drawing it at its own.
	tb.conn.claiming(true)
	waitFor(t, "the second takeover to hold the lease", 20*time.Second, func() bool {
		w.drain()
		return tb.held()
	})
	waitFor(t, "the shell to take the pane's size again", 20*time.Second, func() bool {
		s, ok := f.Hub().Session(shell)
		if !ok {
			return false
		}
		sum := s.Summary()
		return sum.Cols == viewCols && sum.Rows == viewRows
	})
}
