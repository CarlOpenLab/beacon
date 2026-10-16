package main

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/settings"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

// window is the desktop window's state: the shells on this machine, one native
// terminal each, and the tab the user is looking at.
//
// The state belongs to the UI thread. The session list is watched from another
// goroutine, but that goroutine only ever publishes through Window.Update, so
// the state is written and read on one thread and needs no lock of its own.
type window struct {
	fleet *fleet
	store *settings.Store
	font  terminal.Font
	shell string
	cwd   string

	// win is the main window handle, kept so a settings change can re-letter
	// the terminals through Update — they belong to this window's UI thread.
	win *mygo.Window

	// settingsWin is the preferences window while it is open, so a second
	// click brings it forward instead of stacking another copy. It is reached
	// from the window-opening goroutine and the close callback, hence the lock.
	settingsMu  sync.Mutex
	settingsWin *mygo.Window

	// order is the tabs in the order they were opened, by key; selected indexes
	// it. A tab is keyed rather than named by session id because a tab exists
	// for a moment before the hub has told the window which session it got.
	order    []int
	tabs     map[int]*tab
	selected int

	// local is the window's own client connection to this machine's hub. Every
	// shell on this machine is watched through it, under the same lease a
	// browser answers to.
	local *localLink

	// remote is one link per machine the window drives through the relay, keyed
	// by that machine's roster id. They are opened lazily: a machine the panel
	// shows but nobody has looked at yet has no stream.
	remote map[string]*relayLink

	// roster is the relay's machine list as it last pushed it, and remoteSessions
	// is what each machine's hub said it was running. Both are the panel's data.
	roster         []tunnel.Machine
	remoteSessions map[string][]protocol.Summary

	// rail is the navigation sidebar's selection: the id of the shell on screen,
	// or one of the rail's action ids on the frame a click is handled.
	rail string

	// sidebarSplit is the height of the shells' panel in the column, which the
	// line between it and the machines' panel drags. A negative value means the
	// line has not been dragged: the shells then take whatever the machines do
	// not need, so the machines' panel is as tall as its content and no taller,
	// whatever height the window is.
	sidebarSplit float32

	// tabSeq numbers the tabs of this window, so two shells of the same kind
	// can be told apart ("zsh 1", "zsh 2") before either reports a title.
	tabSeq  int
	nextKey int

	// leaseOpen is whether the popover over a taken-over shell is showing. It
	// is the window's own state, so the panel stays where the user left it.
	leaseOpen bool

	// posted is what the links have reported and the UI thread has not applied
	// yet. Everything the links say reaches the window's state through it, so
	// that state belongs to one thread: the UI thread in the app, and whichever
	// thread drains in a test.
	postedMu sync.Mutex
	posted   []func()
}

func newWindow(f *fleet, store *settings.Store, shell, cwd string) *window {
	w := &window{
		fleet:        f,
		store:        store,
		font:         termFont(store.Get()),
		shell:        shell,
		cwd:          cwd,
		tabs:         make(map[int]*tab),
		sidebarSplit: -1,
		nextKey:      1,
	}
	// The window is a client of its own hub from the moment it opens, so every
	// shell it shows — including one a browser took over — arrives as frames
	// and answers to the hub's arbitration.
	w.local = openLocalLink(f.Hub())
	w.local.onSessions = w.sessionsArrived
	w.local.onGone = w.linkGone
	w.remote = map[string]*relayLink{}
	w.remoteSessions = map[string][]protocol.Summary{}
	// The relay's roster reaches the panel through the fleet's client link, and
	// keeps reaching it when the settings window remakes that link.
	f.onRoster = w.rosterArrived
	if c := f.RelayClient(); c != nil {
		c.OnRoster = w.rosterArrived
	}
	return w
}

// update posts fn to the window's own thread. Everything the links report comes
// from their reader goroutines and reaches the window's state this way: the UI
// thread in the app, and whichever thread drains in a test.
func (w *window) update(fn func()) {
	w.postedMu.Lock()
	w.posted = append(w.posted, fn)
	win := w.win
	w.postedMu.Unlock()
	if win != nil {
		win.Update(w.drain)
	}
}

// drain applies everything the links have posted. It runs on the UI thread in
// the app; a test calls it by hand, which is what makes the posted state visible
// to the test's own reads.
func (w *window) drain() {
	w.postedMu.Lock()
	fns := w.posted
	w.posted = nil
	w.postedMu.Unlock()
	for _, fn := range fns {
		fn()
	}
	w.invalidate()
}

// invalidate asks for a repaint. The ui is immediate-mode: the next frame
// draws the window's state as it now stands.
func (w *window) invalidate() {
	if w.win != nil {
		w.win.Invalidate()
	}
}

// sessionsArrived is the hub's session list, pushed on every change: a shell
// started from anywhere — the New Tab button here, or a browser driving this
// machine — turns up the same way.
func (w *window) sessionsArrived(list []protocol.Summary) {
	w.update(func() { w.reconcile(list) })
}

// rosterArrived is the relay pushing its machine list. The panel redraws from
// it; the streams themselves are opened by looking at a machine, so a machine
// nobody has asked about costs nothing.
func (w *window) rosterArrived(machines []tunnel.Machine) {
	w.update(func() {
		w.roster = machines
		w.invalidate()
	})
}

// remoteSessionsArrived is a remote machine's hub reporting what it runs.
func (w *window) remoteSessionsArrived(machine string, list []protocol.Summary) {
	w.update(func() {
		w.remoteSessions[machine] = list
		w.invalidate()
	})
}

// linkFor is the link that speaks to a machine: this machine's own, or another
// machine's through the relay.
func (w *window) linkFor(machine string) *link {
	if machine == machineLocal || machine == "" {
		return w.local.link
	}
	if l := w.remote[machine]; l != nil {
		return l.link
	}
	return nil
}

// remoteLink opens (or reuses) the window's link to a machine, and asks that
// machine's hub what it is running.
func (w *window) remoteLink(machine string) *relayLink {
	if l := w.remote[machine]; l != nil {
		return l
	}
	c := w.fleet.RelayClient()
	if c == nil {
		return nil
	}
	l := openRelayLink(c, machine)
	l.onSessions = func(list []protocol.Summary) { w.remoteSessionsArrived(machine, list) }
	l.onGone = func(reason string) {
		w.update(func() {
			delete(w.remote, machine)
			delete(w.remoteSessions, machine)
			log.Printf("tfdesktop: %s: %s", machine, reason)
			w.invalidate()
		})
	}
	w.remote[machine] = l
	return l
}

// linkGone is the window's own connection to its hub ending.
func (w *window) linkGone(reason string) {
	w.update(func() {
		log.Printf("tfdesktop: %s", reason)
		w.invalidate()
	})
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

	// The appearance is a saved preference, and the ui reads the theme once per
	// frame, so every frame of every window asks for it.
	mode := w.store.Get().Theme
	applyTheme(c, mode)

	ui.Row(c).Fill().Background(c.Theme().Background).Children(func() {
		w.activityRail(c, mode)
		w.sidebar(c, labels, mode)
		ui.Column(c).Fill().Children(func() {
			w.header(c, labels)
			w.body(c, labels)
			w.statusBar(c)
		})
	})
}

// newTab asks the hub for a shell and gives it a tab to land in. It runs on the
// window's own thread — the tab is window state, and a goroutine that opened one
// would race the frame that draws it — and the frame itself does not repaint, so
// there is nothing to defer. The hub's answer arrives as a `created` frame and
// adopts the tab that is waiting for it.
func (w *window) newTab() {
	w.openTab(machineLocal, "")
	w.local.create(80, 24, w.cwd, w.shell)
}

// kill ends the shell behind a tab. The tab itself goes when the hub reports
// the session gone.
func (w *window) kill(id string) {
	w.local.closeSession(id)
}

// reconcile brings the tabs in line with the sessions the hub reports. It runs
// on the UI thread.
func (w *window) reconcile(live []protocol.Summary) {
	present := make(map[string]bool, len(live))
	for _, s := range live {
		present[s.SessionID] = true
	}

	// A shell that ended takes its tab with it.
	for key, tb := range w.tabs {
		if tb.id == "" || present[tb.id] {
			continue
		}
		tb.close()
		delete(w.tabs, key)
		w.order = dropKey(w.order, key)
	}

	// A shell that appeared gets one.
	for _, s := range live {
		if w.tabOf(machineLocal, s.SessionID) != nil {
			continue
		}
		if err := w.mount(s); err != nil {
			log.Printf("tfdesktop: tab for %s: %v", s.SessionID, err)
		}
	}

	if w.selected >= len(w.order) {
		w.selected = len(w.order) - 1
	}
	if w.selected < 0 {
		w.selected = 0
	}
}

// mount gives a session its native terminal. The terminal reads and writes the
// session through the window's link, so it draws the real shell — the same one
// a browser sees — and types into it under the same lease a browser answers to.
func (w *window) mount(s protocol.Summary) error {
	tb := w.openTab(machineLocal, s.SessionID)
	if s.Title != "" {
		tb.setTitle(s.Title)
	} else {
		// The shell has not named itself yet, so label the tab by what it is
		// rather than by its session id.
		w.tabSeq++
		tb.ordinal = w.tabSeq
		tb.program = programName(s.Shell)
	}
	return nil
}

// openTab gives the window a new tab watching a session on machine, attaching
// to it over that machine's link. A session id of "" is one the window has just
// asked the hub to create: the tab waits for the hub to name it.
func (w *window) openTab(machine, sessionID string) *tab {
	l := w.linkFor(machine)
	if l == nil {
		// No link to that machine yet: looking at it opens one.
		if w.remoteLink(machine) != nil {
			l = w.linkFor(machine)
		}
	}
	if l == nil {
		l = w.local.link
	}
	sess := newSession(sessionID, l, nil)
	conn := newTermConn(sess)
	sess.handlers = tabHandlers{w: w, sess: sess, conn: conn}
	term, err := terminal.New(terminal.Options{
		Conn:    conn,
		OnTitle: func(string) {},
		OnExit:  func(int) {},
		// The font saved in settings, so a tab opened after a change — or on a
		// later run — is lettered the same as the ones already on screen.
		Font: w.font,
		// The shell's own colours, so the pane belongs to the window around it.
		Theme:     terminalTheme(false),
		DarkTheme: terminalTheme(true),
	})
	if err != nil {
		log.Printf("tfdesktop: terminal: %v", err)
	}
	key := w.nextKey
	w.nextKey++
	tb := &tab{
		id:      sessionID,
		term:    term,
		conn:    conn,
		sess:    sess,
		machine: machine,
	}
	w.tabs[key] = tb
	w.order = append(w.order, key)
	w.selected = len(w.order) - 1

	if sessionID != "" {
		sess.bind(sessionID)
		l.attach(sessionID, 0)
	} else {
		// The hub's answer names this session; until it arrives the link holds
		// the tab's place.
		l.watch(sess)
	}
	return tb
}

// removeTab takes a tab away. The shell is left alone: ending it is the hub's
// job, so closing a tab here never takes the shell away from a browser still
// attached to it. Nothing is sent — the hub forgets a watcher when its session
// goes away, and a detach here would broadcast a session list that still holds
// a shell that is on its way out, which the next frame would answer by opening
// a tab for it.
func (w *window) removeTab(tb *tab) {
	key := -1
	for k, t := range w.tabs {
		if t == tb {
			key = k
			break
		}
	}
	if key < 0 {
		return
	}
	tb.close()
	delete(w.tabs, key)
	w.order = dropKey(w.order, key)
	if w.selected >= len(w.order) {
		w.selected = len(w.order) - 1
	}
	if w.selected < 0 {
		w.selected = 0
	}
}

// tabOfSession is the tab a session belongs to, found through the session
// rather than its id: a session the hub has just named has a tab whose id the
// window is only now learning.
func (w *window) tabOfSession(s *session) *tab {
	for _, tb := range w.tabs {
		if tb.sess == s {
			return tb
		}
	}
	return nil
}

// tabOf is the tab watching a session on a machine, or nil.
func (w *window) tabOf(machine, sessionID string) *tab {
	if sessionID == "" {
		return nil
	}
	for _, tb := range w.tabs {
		if tb.machine == machine && tb.id == sessionID {
			return tb
		}
	}
	return nil
}

// tabOfID is the tab watching a session id, on any machine.
func (w *window) tabOfID(id string) *tab {
	if id == "" {
		return nil
	}
	for _, tb := range w.tabs {
		if tb.id == id {
			return tb
		}
	}
	return nil
}

// activeTab is the tab on screen, or nil with none open.
func (w *window) activeTab() *tab {
	if w.selected < 0 || w.selected >= len(w.order) {
		return nil
	}
	return w.tabs[w.order[w.selected]]
}

// dropKey removes one key from a slice of them, keeping the rest in order.
func dropKey(keys []int, key int) []int {
	kept := keys[:0]
	for _, k := range keys {
		if k != key {
			kept = append(kept, k)
		}
	}
	return kept
}

// machineLocal is the roster id of the machine this window runs on. Every other
// machine is reached through the relay, which the Reach phase adds; until then
// this machine is the only one there is.
const machineLocal = "local"

// tabHandlers routes one session's frames to its tab. They run on the link's
// reader goroutine, so everything that touches window state goes through
// update, and a change asks for a repaint: the window is immediate-mode, and
// the next frame draws whatever the lease now says.
type tabHandlers struct {
	w    *window
	sess *session
	conn *termConn
}

func (h tabHandlers) tab() *tab { return h.w.tabOfID(h.sess.id) }

func (h tabHandlers) Attached(sum protocol.Summary, cursor int64, truncated bool) {
	w := h.w
	w.update(func() {
		tb := h.tab()
		if tb == nil {
			return
		}
		tb.alive = sum.Alive
		if sum.Title != "" {
			tb.setTitle(sum.Title)
		}
		w.invalidate()
	})
}

func (h tabHandlers) Created(id string) {
	w := h.w
	w.update(func() {
		// The tab that asked for this shell is the one watching it now: its
		// label, its pane and its lease all belong to the same tab.
		if tb := w.tabOfSession(h.sess); tb != nil {
			tb.bind(id)
		}
		w.invalidate()
	})
}

func (h tabHandlers) Output(data []byte, resync bool) {
	h.conn.feed(data, resync)
}

func (h tabHandlers) Role(role, controller string, epoch uint64) {
	w := h.w
	w.update(func() {
		if tb := h.tab(); tb != nil {
			tb.setLease(role, controller, epoch)
		}
		w.invalidate()
	})
}

func (h tabHandlers) Exit(code int) {
	w := h.w
	// The line goes straight to the terminal: the hub has already flushed the
	// shell's last output, and this is the window's own word on the end.
	h.conn.feed([]byte(fmt.Sprintf("\r\n\x1b[2m[进程已退出，退出码 %d]\x1b[0m\r\n", code)), false)
	w.update(func() {
		if tb := h.tab(); tb != nil {
			tb.finish(code)
		}
		w.invalidate()
	})
}

func (h tabHandlers) Closed() {
	w := h.w
	w.update(func() {
		if tb := h.tab(); tb != nil {
			w.removeTab(tb)
		}
		w.invalidate()
	})
}

func (h tabHandlers) Gone(reason string) {
	w := h.w
	w.update(func() {
		if tb := h.tab(); tb != nil {
			w.removeTab(tb)
		}
		log.Printf("tfdesktop: %s", reason)
		w.invalidate()
	})
}

func (h tabHandlers) Failed(code, message string) {
	// A refused keystroke and a refused claim are answers, not failures: the
	// popover over the terminal says the same thing on screen, and the log
	// keeps the detail for when it is needed.
	log.Printf("tfdesktop: %s: %s", code, message)
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
	conn *termConn
	sess *session

	// machine is the roster id this shell runs on: machineLocal for this
	// machine, or another machine's id when the shell was taken over through
	// the relay.
	machine string

	// alive is what the hub last said about the shell: false once it has
	// exited, which is what marks the tab dead.
	alive bool

	mu    sync.Mutex
	title string
	// program and ordinal are the fallback label for a shell that has not
	// reported a title of its own.
	program string
	ordinal int
	done    bool
	exit    int

	// lease is what the hub last said about who may type: this window's role,
	// who holds the lease when it is not this window, and the epoch.
	role       string
	controller string
	epoch      uint64
}

// bind gives the tab its session id, once the hub has named it. Until then the
// tab is one the window has asked for and not yet been told about.
func (t *tab) bind(id string) {
	t.mu.Lock()
	t.id = id
	t.mu.Unlock()
	t.sess.bind(id)
}

// held reports whether this window may type into the shell: it holds the lease.
// The glow and the popover are drawn from the same answer.
func (t *tab) held() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.role == protocol.RoleController
}

// holder is who holds the lease when this window does not.
func (t *tab) holder() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.role == protocol.RoleController {
		return ""
	}
	return t.controller
}

// setLease records a role frame.
func (t *tab) setLease(role, controller string, epoch uint64) {
	t.mu.Lock()
	t.role, t.controller, t.epoch = role, controller, epoch
	t.mu.Unlock()
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
	name := t.title
	if name == "" {
		// Nothing named this shell yet. Say what the tab is instead of
		// showing a session id, which tells the reader nothing.
		switch {
		case t.program != "":
			name = fmt.Sprintf("%s %d", t.program, t.ordinal)
		case t.ordinal > 0:
			name = fmt.Sprintf("终端 %d", t.ordinal)
		default:
			name = t.id
		}
	}
	if t.done {
		return fmt.Sprintf("%s (%d)", name, t.exit)
	}
	return name
}

// programName turns the shell path a session reports into the short name shown
// on a tab: "/bin/zsh" is "zsh" and `C:\...\powershell.exe` is "PowerShell".
// Both separators are handled because a client may show a session running on
// another operating system.
func programName(path string) string {
	if path == "" {
		return ""
	}
	base := path
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	if strings.EqualFold(base, "powershell") {
		return "PowerShell"
	}
	return base
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

// serviceLabel is what the toolbar says about the local service: its address
// while it is listening, and the fact that it is off when a setting turned it
// off. The window keeps working either way.
func (w *window) serviceLabel() string {
	if url := w.fleet.BaseURL(); url != "" {
		return url
	}
	return "本机服务已关闭"
}

// reletter re-letters every open terminal. The settings window calls it, and it
// goes through Update so the terminals — which belong to this window's UI
// thread — are never touched from another one. It also becomes the font of the
// tabs opened afterwards.
func (w *window) reletter(f terminal.Font) {
	win := w.win
	if win == nil {
		return
	}
	win.Update(func() {
		w.font = f
		for _, tb := range w.tabs {
			if tb.term != nil {
				tb.term.SetFont(f)
			}
		}
	})
}

// invalidateSettings redraws the settings window if it is open. The relay link
// reports its state from the agent's goroutine, and an immediate-mode pane only
// repaints when something tells it to.
func (w *window) invalidateSettings() {
	w.settingsMu.Lock()
	win := w.settingsWin
	w.settingsMu.Unlock()
	if win != nil {
		win.Invalidate()
	}
}

// openSettings shows the preferences window, or brings the open one forward.
// It runs off the UI thread, as New Tab does, so a click never re-enters the
// render pass that drew the button.
func (w *window) openSettings() {
	w.settingsMu.Lock()
	defer w.settingsMu.Unlock()
	if w.settingsWin != nil {
		w.settingsWin.Focus()
		return
	}
	sw := newSettingsWindow(w, w.store, w.fleet)
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:           "term-fleet 设置",
		Width:           540,
		Height:          440,
		MinWidth:        440,
		MinHeight:       360,
		BackgroundColor: windowGround(w.store.Get().Theme),
		StateKey:        "settings",
		Content:         ui.View(sw.view),
	})
	sw.win = win
	w.settingsWin = win
	win.OnClosed(func() {
		w.settingsMu.Lock()
		w.settingsWin = nil
		w.settingsMu.Unlock()
	})
}
