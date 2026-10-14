package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/daemon"
	"github.com/CarlOpenLab/term-fleet/internal/native"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
)

// window is the desktop window's state: the shells on this machine, one native
// terminal each, and the tab the user is looking at.
//
// The state belongs to the UI thread. The session list is watched from another
// goroutine, but that goroutine only ever publishes through Window.Update, so
// the state is written and read on one thread and needs no lock of its own.
type window struct {
	srv   *daemon.Server
	shell string
	cwd   string

	order    []string // session ids, in tab order
	tabs     map[string]*tab
	selected int

	// Signature of the session list as last published. The watch loop below
	// compares against it so an unchanged list costs no repaint; it is the one
	// field two goroutines share.
	sig atomic.Value // string

	pokes chan struct{} // a nudge to the watch loop, as after New Tab
	focus atomic.Value  // string: the session to select once it appears
}

func newWindow(srv *daemon.Server, shell, cwd string) *window {
	w := &window{
		srv:   srv,
		shell: shell,
		cwd:   cwd,
		tabs:  make(map[string]*tab),
		pokes: make(chan struct{}, 1),
	}
	w.sig.Store("")
	w.focus.Store("")
	return w
}

// view draws the window: a toolbar, a tab per shell, and the shell the user is
// looking at — drawn by mygo's own terminal, not a webview.
func (w *window) view(c *ui.Context) {
	labels := make([]string, 0, len(w.order))
	for _, id := range w.order {
		if tb := w.tabs[id]; tb != nil {
			labels = append(labels, tb.label())
		}
	}
	if w.selected >= len(labels) {
		w.selected = len(labels) - 1
	}
	if w.selected < 0 {
		w.selected = 0
	}

	ui.Column(c).Fill().Children(func() {
		ui.Toolbar(c, func() {
			if ui.Button(c, "New Tab").Clicked() {
				go w.create()
			}
			if len(labels) > 0 && ui.Button(c, "Close Tab").Clicked() {
				// Read the selection here, on the UI thread, and hand the
				// goroutine an id rather than a position: the list can change
				// while it works.
				id := w.order[w.selected]
				go w.kill(id)
			}
			ui.Spacer(c)
			// The address a browser on this machine can use to reach the same
			// shells, which is the point of the daemon sitting underneath.
			ui.Text(c, w.srv.BaseURL())
		})

		if len(labels) == 0 {
			ui.Text(c, "No shells yet. New Tab starts one.")
			return
		}
		ui.Tabs(c, &w.selected, labels...)
		if tb := w.tabs[w.order[w.selected]]; tb != nil {
			terminal.View(c, tb.term).Fill().AutoFocus()
		}
	})
}

// create starts a shell, which the watch loop then mounts as a new tab. It
// goes through the daemon's hub rather than around it, so a browser attached
// to the same hub sees this tab appear as it does its own.
func (w *window) create() {
	s, err := w.srv.Hub().Create("", 80, 24, w.cwd, w.shell)
	if err != nil {
		log.Printf("tfdesktop: new tab: %v", err)
		return
	}
	w.focus.Store(s.ID)
	w.poke()
}

// kill ends the shell behind a tab. The tab itself goes when the watch loop
// sees the session disappear.
func (w *window) kill(id string) {
	if err := w.srv.Hub().Kill(id); err != nil {
		log.Printf("tfdesktop: close tab: %v", err)
		return
	}
	w.poke()
}

func (w *window) poke() {
	select {
	case w.pokes <- struct{}{}:
	default:
	}
}

// watch mirrors this machine's sessions into tabs. It polls rather than
// subscribing so that a shell started from anywhere — the New Tab button here,
// or a browser driving this machine — turns up the same way.
func (w *window) watch(win *mygo.Window) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
		case <-w.pokes:
		}
		live := w.srv.Hub().Sessions()
		if signature(live) == w.published() {
			continue
		}
		win.Update(func() { w.reconcile(live) })
	}
}

// reconcile brings the tabs in line with the sessions the hub reports. It runs
// on the UI thread.
func (w *window) reconcile(live []protocol.Summary) {
	present := make(map[string]bool, len(live))
	for _, s := range live {
		present[s.SessionID] = true
	}

	// A shell that ended takes its tab with it.
	for id, tb := range w.tabs {
		if present[id] {
			continue
		}
		tb.close()
		delete(w.tabs, id)
		w.order = dropID(w.order, id)
	}

	// A shell that appeared gets one.
	for _, s := range live {
		if _, ok := w.tabs[s.SessionID]; ok {
			continue
		}
		tb, err := w.mount(s)
		if err != nil {
			log.Printf("tfdesktop: tab for %s: %v", s.SessionID, err)
			continue
		}
		w.tabs[s.SessionID] = tb
		w.order = append(w.order, s.SessionID)
		if w.focus.Load() == s.SessionID {
			w.selected = len(w.order) - 1
			w.focus.Store("")
		}
	}

	if w.selected >= len(w.order) {
		w.selected = len(w.order) - 1
	}
	if w.selected < 0 {
		w.selected = 0
	}
	w.sig.Store(signature(live))
}

// mount gives a session its native terminal. The terminal reads and writes the
// session through native.Conn, so it draws the real shell — the same one a
// browser sees — and types into it.
func (w *window) mount(s protocol.Summary) (*tab, error) {
	sess, ok := w.srv.Hub().Session(s.SessionID)
	if !ok {
		return nil, fmt.Errorf("session is gone")
	}
	tb := &tab{id: s.SessionID, conn: native.NewConn(sess), title: s.Title}
	if tb.title == "" {
		tb.title = s.SessionID
	}
	term, err := terminal.New(terminal.Options{
		Conn:    tb.conn,
		OnTitle: tb.setTitle,
		OnExit:  tb.finish,
	})
	if err != nil {
		_ = tb.conn.Close()
		return nil, err
	}
	tb.term = term
	return tb, nil
}

func (w *window) published() string {
	s, _ := w.sig.Load().(string)
	return s
}

// signature is a cheap fingerprint of the session list: enough to tell a
// change worth a repaint from the same list seen again.
func signature(list []protocol.Summary) string {
	var b strings.Builder
	for _, s := range list {
		fmt.Fprintf(&b, "%s/%s/%d/%d/%t;", s.SessionID, s.Title, s.Cols, s.PID, s.Alive)
	}
	return b.String()
}

func dropID(ids []string, id string) []string {
	kept := ids[:0]
	for _, v := range ids {
		if v != id {
			kept = append(kept, v)
		}
	}
	return kept
}

// tab is one session seen natively: its terminal, and the little the window
// needs to label it. The title and the exit arrive on the terminal's own
// goroutines, so they are guarded here; the terminal itself is used only from
// the UI thread.
type tab struct {
	id   string
	term *terminal.Terminal
	conn *native.Conn

	mu    sync.Mutex
	title string
	done  bool
	exit  int
}

func (t *tab) setTitle(title string) {
	if title == "" {
		return
	}
	t.mu.Lock()
	t.title = title
	t.mu.Unlock()
}

func (t *tab) finish(code int) {
	t.mu.Lock()
	t.done = true
	t.exit = code
	t.mu.Unlock()
}

func (t *tab) label() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return fmt.Sprintf("%s (%d)", t.title, t.exit)
	}
	return t.title
}

// close releases the terminal and detaches the view. The shell is left alone:
// ending it is the hub's job, so that closing a tab here never takes the shell
// away from a browser still attached to it.
func (t *tab) close() {
	if t.term != nil {
		_ = t.term.Close()
	}
	_ = t.conn.Close()
}
