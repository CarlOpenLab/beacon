package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/diag"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/settings"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
	"github.com/CarlOpenLab/term-fleet/locales"
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

	// frameStub stands in for win in a test that draws the window's own
	// controls: the app always has the real window, and a headless tester has
	// none. See frame.go.
	frameStub windowFrame

	settingsContent *settingsWindow
	settingsOpen    bool
	// settingsDrag is where the settings dialog has been dragged to, from the
	// centre of the window: the dialog is a floating one, not a sheet over the
	// whole page, so it can be moved aside to watch what a setting changes.
	settingsDrag  ui.Point
	sidebarHidden bool
	dismissed     map[string]bool

	// chrome is where the sidebar's fold has the column and the cards this
	// frame; see sidebar.go. The view writes it and the parts it draws read it.
	chrome sideChrome

	// sidebarWidth, sideDrag and collapsed are the column's own state: how wide
	// it is (0 for the default), the width a drag started from, and which
	// machine groups the person folded away.
	sidebarWidth float32
	sideDrag     float32
	collapsed    map[string]bool

	// zoomed fills the body with the focused pane of a split, ⌘⇧↩.
	zoomed bool

	// pal is the command palette's state.
	pal paletteState

	// statusSeen is the state each session was in when its machine last pushed
	// a list, keyed as "machine\x00session". A notification is about a change,
	// so a session that was already waiting when the window opened says nothing.
	statusSeen map[string]string

	// onNotify, when set (by a test), receives what would have been shown as a
	// notification. The decision is the part worth testing; the banner is the
	// desktop's own business.
	onNotify func(title, body string)

	// order is the tabs in the order they were opened, by key; selected indexes
	// it. A tab is keyed rather than named by session id because a tab exists
	// for a moment before the hub has told the window which session it got.
	order    []int
	tabs     map[int]*tab
	selected int

	// split is the active workspace's tree of panes, and nil when the body is
	// the one pane the strip selects — the ordinary case, and the one every path
	// below keeps when there is no split. focus is the leaf of that tree the
	// window acts on; it is a leaf whenever split is not nil. ws is the workspace
	// those pointers belong to. See split.go.
	split *splitNode
	focus *splitNode
	ws    *workspace

	// renaming is the shell whose name is being edited and renameDraft is what
	// the field holds; see rename.go. renameSeed is the name the draft started
	// as — what the row showed — and renameArmed says the field has been on
	// screen for a frame, which is what lets a press elsewhere commit it.
	renaming    *renameTarget
	renameDraft string
	renameSeed  string
	renameArmed bool

	// cmdBar is the command composer and its history: what this window has typed
	// into the shell on screen, and the box it is typed into again.
	cmdBar *commandBar

	// local is the window's own client connection to this machine's hub. Every
	// shell on this machine is watched through it, under the same lease a
	// browser answers to.
	local *localLink

	// remote is one link per machine the window drives through the relay, keyed
	// by that machine's roster id. They are opened lazily: a machine the panel
	// shows but nobody has looked at yet has no stream.
	remote map[string]*relayLink

	// refreshAt is when each machine's session list was last asked for, keyed by
	// machine id, so the group's refresh can turn from the click to the answer.
	// See refreshing.
	refreshAt map[string]time.Time

	// roster is the relay's machine list as it last pushed it, and remoteSessions
	// is what each machine's hub said it was running. Both are the panel's data.
	roster         []tunnel.Machine
	remoteSessions map[string][]protocol.Summary
	// live is this machine's own session list as the hub last pushed it, with who
	// holds each shell. The panel counts the shells other clients are driving from
	// it, which is the question a person asks when their typing goes nowhere.
	live []protocol.Summary
	// linkNote is why the window could not reach a machine just now, shown in
	// the panel: a click that does nothing is the one thing a fleet panel must
	// never do.
	linkNote string

	// machineSel is the machine the 机器 panel is showing: machineLocal, or a
	// roster id. The 会话 panel below it is that machine's shells, and the panel's
	// 新建标签页 opens one there.
	machineSel string

	// reachPrompt is the machine a person picked that this window cannot reach
	// yet, which is what puts the "ask the relay for a credential" dialog on
	// screen; reachErr is why the last attempt did not work.
	reachPrompt string
	reachErr    string

	// notice is the window's own line about something that just happened — a
	// machine taking its shell back — and noticeAt is when it was said, so it can
	// fade on its own.
	notice   string
	noticeAt time.Time

	// arranged is the tab whose lease the window has already settled while it is
	// the shell on screen, so the settling happens on the way in and not every
	// frame (see followLease).
	arranged *tab

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
		collapsed:    make(map[string]bool),
		sidebarSplit: -1,
		nextKey:      1,
		cmdBar:       newCommandBar(),
		dismissed:    make(map[string]bool),
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
		c.SetOnRoster(w.rosterArrived)
		// The roster is pushed on presence changes, not to whoever subscribes: a
		// window that opens after the relay has already said who is here has to be
		// told, or the panel stays empty until the next machine comes or goes.
		if machines := c.Machines(); len(machines) > 0 {
			w.rosterArrived(machines)
		}
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
	w.update(func() {
		w.refreshLanded(machineLocal)
		w.reconcile(list)
		w.followSizes(machineLocal, list)
		w.reportStatuses(machineLocal, list)
		w.updateTitle()
	})
}

// reportStatuses tells the user — with a notification, and with the Dock badge
// — what changed while they were looking elsewhere. It runs on the window's
// thread, from the list each machine pushed, and it is the only place a
// notification is made: the rule that a session coming to need you, failing
// or finishing unseen is news, and everything else is the window's own business.
func (w *window) reportStatuses(machine string, list []protocol.Summary) {
	if w.statusSeen == nil {
		w.statusSeen = map[string]string{}
	}
	for i := range list {
		s := list[i]
		key := machine + "\x00" + s.SessionID
		prev, seen := w.statusSeen[key]
		w.statusSeen[key] = s.Status
		if !seen || prev == s.Status {
			continue
		}
		// The session on screen, in a window the person is at, is not news.
		if w.win != nil && w.win.IsFocused() && !w.sidebarHidden {
			if tb := w.tabOf(machine, s.SessionID); tb != nil && tb == w.activeTab() {
				continue
			}
		}
		title, body, ok := w.noticeFor(prev, s)
		if !ok {
			continue
		}
		w.notify(machine, s.SessionID, title, body)
	}
	w.refreshBadge()
}

// noticeFor is what a change is worth telling: a session that comes to need the
// user, a program that reports a failure, and an agent that finishes while
// nobody is looking. Everything else is the window's own business.
func (w *window) noticeFor(prev string, s protocol.Summary) (title, body string, ok bool) {
	set := w.store.Get()
	switch {
	case s.Status == "waiting" && !set.NotifyWaitingOff:
		title, body = locales.NotifyNeedsYouTitle(sessionLabel(s)), s.Message
		if body == "" {
			switch s.Kind {
			case "permission":
				body = locales.StatusNeedsPermission()
			case "question":
				body = locales.StatusHasQuestion()
			case "auth":
				body = locales.StatusNeedsSignIn()
			default:
				body = locales.NotifyBodyWaiting()
			}
		}
	case s.Status == "error" && !set.NotifyErrorOff:
		title, body = locales.NotifyFailedTitle(sessionLabel(s)), s.Message
		if body == "" {
			body = locales.NotifyBodyError()
		}
	case s.Status == "idle" && prev == "working" && s.Unread && !set.NotifyDoneOff:
		title, body = locales.NotifyDoneTitle(sessionLabel(s)), s.Message
		if body == "" {
			body = locales.NotifyBodyDone()
		}
	default:
		return "", "", false
	}
	return title, body, true
}

// notify shows one notification, whose click puts that session on screen.
func (w *window) notify(machine, id, title, body string) {
	if w.onNotify != nil {
		w.onNotify(title, body)
		return
	}
	if w.win == nil || !mygo.NotificationsSupported() {
		return
	}
	n := mygo.NewNotification(mygo.NotificationOptions{
		ID:    "session/" + machine + "/" + id,
		Group: id,
		Title: title,
		Body:  body,
	})
	n.OnClick(func() {
		w.update(func() {
			if tb := w.tabOf(machine, id); tb != nil {
				delete(w.collapsed, machine)
				w.selectTab(tb)
			} else {
				w.openOrSelect(machine, id)
			}
			if w.win != nil {
				w.win.Show()
				w.win.Focus()
			}
			mygo.App.Focus()
		})
		n.Close()
	})
	go n.Show()
}

// refreshBadge puts how many sessions need the user on the Dock icon, or clears
// it when the person turned that off or nothing needs them.
func (w *window) refreshBadge() {
	if w.win == nil {
		return
	}
	n := 0
	if !w.store.Get().DockBadgeOff {
		count := func(list []protocol.Summary) {
			for _, s := range list {
				if s.Status == "waiting" {
					n++
				}
			}
		}
		count(w.live)
		for _, list := range w.remoteSessions {
			count(list)
		}
	}
	mygo.App.SetBadgeCount(n)
}

// sessionLabel is a session's name for a notification, from what the list says:
// its title, else the last part of its directory, else its id.
func sessionLabel(s protocol.Summary) string {
	name := strings.TrimSpace(s.Title)
	if name == "" {
		name = filepath.Base(strings.TrimSpace(s.Cwd))
	}
	if name == "" || name == "." || name == "/" {
		name = s.SessionID
	}
	return name
}

// zoomText steps the terminal's size, as ⌘= and ⌘- do, or puts it back to the
// default with ⌘0. The store keeps the choice, so the next window opens at it.
func (w *window) zoomText(delta int) {
	size := w.store.Get().FontSize
	if size <= 0 {
		size = settings.DefaultFontSize
	}
	switch {
	case delta == 0:
		size = settings.DefaultFontSize
	case delta > 0:
		size = min(size+1, 32)
	default:
		size = max(size-1, 8)
	}
	if _, err := w.store.Update(func(st *settings.Settings) { st.FontSize = size }); err != nil {
		log.Printf("tfdesktop: save font size: %v", err)
		return
	}
	w.reletter(termFont(w.store.Get()))
	w.invalidate()
}

// updateTitle says, in the window's own title, what the fleet wants: how many
// sessions need the user, then how many are working. It is the one place a
// person sees the count with the window's chrome out of the way.
func (w *window) updateTitle() {
	if w.win == nil {
		return
	}
	waiting, working, _ := w.statusCounts()
	title := "term-fleet"
	switch {
	case waiting > 0:
		title = fmt.Sprintf("● %d · term-fleet", waiting)
	case working > 0:
		title = fmt.Sprintf("◐ %d · term-fleet", working)
	}
	w.win.SetTitle(title)
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
		w.refreshLanded(machine)
		w.remoteSessions[machine] = list
		// Each tab on that machine keeps the directory its hub last reported,
		// which is where a shell opened beside it falls back to.
		for _, s := range list {
			if tb := w.tabOf(machine, s.SessionID); tb != nil {
				tb.cwd = s.Cwd
			}
		}
		w.reportStatuses(machine, list)
		w.followSizes(machine, list)
		w.invalidate()
	})
}

// noLinkReason is why the window has no link to a machine, in the words the
// panel and the log should use.
func (w *window) noLinkReason(machine string) string {
	if machine == machineLocal || machine == "" {
		return locales.AppLinkNotReady()
	}
	if !w.relayOn() {
		return locales.AppNotEnrolled()
	}
	if w.fleet.RelayClientErr() != "" {
		return locales.AppClientLinkFailed(w.fleet.RelayClientErr())
	}
	return locales.AppNoClientToken()
}

// machineOnline is whether the roster says this machine answers right now.
// This machine always does: its hub is in this process.
//
// It is what stops a session row left over from a machine that has gone from
// opening a tab nothing can ever paint. The panel draws the last session list a
// machine pushed, and that list stays after the machine leaves; a click on it is
// a reasonable thing to do, so the answer has to be a sentence rather than a
// white pane.
func (w *window) machineOnline(machine string) bool {
	if machine == "" || machine == machineLocal {
		return true
	}
	m, ok := w.machineByID(machine)
	return ok && m.Online
}

// linkFor is the link that speaks to a machine: this machine's own, or another
// machine's through the relay.
func (w *window) linkFor(machine string) *link {
	if machine == machineLocal || machine == "" {
		if w.local == nil {
			return nil
		}
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
	l.onOpen = func() {
		// The stream is routed: this link is the machine's as it now stands, so
		// the tabs the previous link left behind are picked back up here —
		// without a click, the way the browser client's reconnect re-attaches
		// its tabs. Open runs on the relay's reader goroutine, so the re-attach
		// goes through the window's thread, which owns the tabs and their
		// terminals.
		w.update(func() { w.reattachMachineTabs(machine, l.link) })
	}
	l.onGone = func(reason string) {
		w.update(func() {
			delete(w.remote, machine)
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

// followLease is what the window does about the shell on screen, and about the
// ones that are not.
//
// A window drives what it is showing, but the lease is not switched by looking:
// the hub gives an unheld shell to the first client that attaches, so opening an
// unheld tab on another machine makes this window its controller. It stays the
// controller when the tab leaves the screen — release is manual (释放) or the
// idle deadline's job, not a side effect of ⌘⇧] — which is why a far machine's
// desktop can be left read-only until somebody hands the keyboard back.
//
// What this function still does is claim a shell of this machine's that nobody
// holds: a holder that lapsed, or a client that let go, must not leave this
// machine's own shell mute. What it never takes is a shell of this machine's
// from another client that holds it — that is 拿回控制权 in the pane, and it
// ends that client's session with this machine's shell, so it stays a person's
// click. The taking happens on the way in rather than every frame, so two
// windows looking at the same shell cannot take it from each other forever.
func (w *window) followLease() {
	on := w.activeTab()

	if on == nil || on.conn == nil || on == w.arranged {
		return
	}
	// The shell on screen has changed: settle its lease once.
	w.arranged = on
	if on.held() {
		return
	}
	if on.machine != machineLocal {
		// Do not automatically take over remote machine shells:
		// watching another machine is read-only by default;
		// taking over (接管) and releasing (释放) are both manual.
		return
	}
	if on.holder() != "" {
		// This machine's own shell, held by another client: taking it back ends
		// that client's session with this machine's shell, so it stays a person's
		// click — 拿回控制权 in the pane — and never a side effect of looking at it.
		return
	}
	on.conn.claiming(false)
}

// view draws the window: a band across the top, a pill per shell, and the shell
// the user is looking at — drawn by mygo's own terminal, not a webview.
func (w *window) view(c *ui.Context) {
	if w.selected >= len(w.order) {
		w.selected = len(w.order) - 1
	}
	if w.selected < 0 {
		w.selected = 0
	}
	w.followLease()

	// The app's chrome, in the appearance the person chose; terminal ANSI keeps its
	// own colors.
	mode := w.store.Get().Theme
	applyTheme(c, mode)

	// Every shortcut comes from the action list, so one rebound in Settings is
	// the one the window reads: see actions.go. ⌘1…⌘9 are the window's own —
	// they name a row, not an action — and are read here.
	w.runShortcuts(c)
	if !w.settingsOpen && w.reachPrompt == "" && !w.pal.open {
		for i, key := range tabDigits {
			if c.Shortcut(ui.Cmd, key) {
				w.jumpTab(i)
				break
			}
		}
	}

	// The window's wash: the root's gradient, and the glow the cards sit in.
	pal := paletteFor(c)
	row := ui.Row(c).Fill().AlignItems(ui.Stretch)
	backdrop(c, pal, row, w.store.Get().Background)
	// The column's fold: one progress drives the sidebar's width, the cards'
	// left padding and the header's inset, so the three move as one surface.
	ch := chromeFor(row.AnimateTo("sidebar", b2f(!w.sidebarHidden), sidebarMotion), w.sideWidth())
	w.chrome = ch
	row.Children(func() {
		if ch.shown > 0.001 {
			w.sidebar(c, pal)
		}
		// The cards' column: the wash shows in the gaps around a card, and the
		// left gap closes when the sidebar is shown, so the column reads as the
		// sidebar's neighbour rather than a second panel.
		ui.Column(c).Grow(1).MinWidth(0).
			Padding(uitokens.CardGap, uitokens.CardGap, uitokens.CardGap, ch.mainPad).DragWindow().Children(func() {
			w.body(c, pal)
		})
		// The column's buttons over the cards while it folds away, so they stay
		// where the pointer found them; the card's header takes their place.
		if ch.shown < 0.75 {
			w.sidebarPeek(c)
		}
		w.settingsDialog(c)
		w.reachDialog(c)
		w.paletteView(c, pal)
		// Last, so the window's own controls are over the dialogs and the
		// palette: a window's close, minimize and maximize stay where they are
		// while one of them is open, as the system's do.
		w.windowControls(c)
	})
}

// loseMachine is what happens when this window's link to a machine is gone for
// good: the tabs it had open there are closed, the link is dropped, the panels
// return to this machine, and the window says why.
//
// This is about the connection, not about who holds a shell. A shell being taken
// back used to come through here too, which closed the stream and removed the
// tab — and with them the 接管 button that would have taken the shell back. A
// lease that moves hands now leaves the link and the tab alone; see tabHandlers.
// What is left here is the machine actually being unreachable.
func (w *window) loseMachine(machine string) {
	name := w.machineName(machine)
	var doomed []*tab
	for _, tb := range w.tabs {
		if tb.machine == machine {
			doomed = append(doomed, tb)
		}
	}
	for _, tb := range doomed {
		w.removeTab(tb)
	}
	if l := w.remote[machine]; l != nil {
		l.close()
		delete(w.remote, machine)
	}
	delete(w.remoteSessions, machine)
	if w.selectedMachine() == machine {
		w.machineSel = machineLocal
	}
	w.leaseOpen = false
	w.setNotice(locales.AppMachineTookBack(name))
}

// noticeTTL is how long the window's own notice stays on screen. Long enough to
// read after looking away from the terminal, short enough not to become part of
// the window.
const noticeTTL = 8 * time.Second

// setNotice puts a line on screen for a while: what just happened, in the
// status bar, where the window already says what the fleet is doing.
func (w *window) setNotice(text string) {
	w.notice, w.noticeAt = text, time.Now()
}

// currentNotice is that line while it is still worth reading, and "" after.
func (w *window) currentNotice() string {
	if w.notice == "" || time.Since(w.noticeAt) > noticeTTL {
		return ""
	}
	return w.notice
}

// selectedMachine is the machine the panels are about: this machine unless the
// person picked another one, and this machine again when the picked one leaves
// the roster — a machine that is gone cannot be the one whose shells are listed.
func (w *window) selectedMachine() string {
	if w.machineSel == "" || w.machineSel == machineLocal {
		return machineLocal
	}
	if _, ok := w.machineByID(w.machineSel); !ok {
		w.machineSel = machineLocal
		return machineLocal
	}
	return w.machineSel
}

// selectMachine picks the machine the panels are about. A machine this window
// cannot reach yet is confirmed first: driving the rest of the fleet means
// asking the relay for a credential that lets this window act as a client, and
// that is a thing a person says yes to once rather than something a click does
// behind their back.
func (w *window) selectMachine(m tunnel.Machine) {
	// Picking a machine shows one of its shells, when this window has one: the
	// panes and the screen are the same subject, and 切到本机 that left the other
	// machine's terminal on screen — still being driven, still showing 正在被控制
	// over there — would be two answers to "what am I working on". The newest
	// shell this window has open there is the one.
	if tb := w.newestTabOn(m.ID); tb != nil {
		w.selectTab(tb)
	}
	if m.ID == machineLocal {
		w.machineSel, w.linkNote = machineLocal, ""
		w.invalidate()
		return
	}
	w.machineSel = m.ID
	w.linkNote = ""
	if w.fleet != nil && w.fleet.RelayClient() == nil {
		w.reachErr = ""
		w.reachPrompt = m.ID
	} else {
		w.ensureSessionList(m.ID)
	}
	w.invalidate()
}

// ensureSessionList asks a machine what it is running, opening the window's link
// to it if there is not one yet. A machine the window cannot reach says why, in
// the panel, rather than leaving the list empty to be guessed at.
func (w *window) ensureSessionList(machine string) {
	if l := w.remoteLink(machine); l != nil {
		w.linkNote = ""
		l.list()
		return
	}
	w.linkNote = w.noLinkReason(machine)
}

// refreshSpin is how long a machine's refresh may turn before its answer is
// given up on. A machine that is off, or one whose stream never comes up, must
// not leave the row turning for the rest of the session; the stream goes on
// retrying on its own, so this is the mark's lifetime rather than the ask's.
const refreshSpin = 8 * time.Second

// refreshRequested marks a machine's list as being asked for, so its group's
// refresh turns from the click until the answer lands.
func (w *window) refreshRequested(machine string) {
	if w.refreshAt == nil {
		w.refreshAt = make(map[string]time.Time)
	}
	if machine == "" {
		machine = machineLocal
	}
	w.refreshAt[machine] = time.Now()
}

// refreshLanded says a machine's list has arrived, so its refresh can stop.
func (w *window) refreshLanded(machine string) {
	if machine == "" {
		machine = machineLocal
	}
	if _, ok := w.refreshAt[machine]; ok {
		delete(w.refreshAt, machine)
		w.invalidate()
	}
}

// refreshing reports whether a machine's list was asked for and has not answered
// yet. It is what the group's refresh turns on.
func (w *window) refreshing(machine string) bool {
	if machine == "" {
		machine = machineLocal
	}
	at, ok := w.refreshAt[machine]
	return ok && time.Since(at) < refreshSpin
}

// refreshAll re-queries the relay for the roster and the active machine for its sessions.
func (w *window) refreshAll() {
	w.refreshMachines()
	w.refreshSessions(w.selectedMachine())
	w.setNotice(locales.AppRefreshedAll())
}

// refreshSessions asks the selected machine's hub for its live session list.
func (w *window) refreshSessions(machine string) {
	// A machine the roster says is gone will not answer, so nothing turns on a
	// click that can only end in a sentence: the sentence is the answer.
	if machine == "" || w.machineOnline(machine) {
		w.refreshRequested(machine)
	}
	if machine == machineLocal || machine == "" {
		if w.local != nil {
			w.local.list()
		}
		w.setNotice(locales.AppRefreshedLocal())
		w.invalidate()
		return
	}
	if l := w.remote[machine]; l != nil && l.stream != nil && l.stream.Closed() {
		delete(w.remote, machine)
	}
	w.ensureSessionList(machine)
	w.setNotice(locales.AppRefreshedMachine(w.machineName(machine)))
	w.invalidate()
}

// refreshMachines asks the relay for the freshest roster of machines.
func (w *window) refreshMachines() {
	if w.fleet == nil {
		return
	}
	if c := w.fleet.RelayClient(); c != nil {
		if machines := c.Machines(); len(machines) > 0 {
			w.roster = machines
		}
		c.RefreshMachines()
	} else if w.relayOn() {
		go func() {
			_ = w.fleet.Reach()
		}()
	}
	w.setNotice(locales.AppRefreshedMachines())
	w.invalidate()
}

// askForReach asks the relay for a short-lived credential and, when one arrives,
// shows the machine that was picked. The request is a network call, so it runs
// off the UI thread and the dialog stays up until the relay answers.
func (w *window) askForReach() {
	machine := w.reachPrompt
	if machine == "" {
		return
	}
	w.reachErr = ""
	w.invalidate()
	go func() {
		err := w.fleet.Reach()
		w.update(func() {
			if err != nil {
				w.reachErr = err.Error()
				w.invalidate()
				return
			}
			w.reachPrompt = ""
			w.ensureSessionList(machine)
			w.invalidate()
		})
	}()
}

// settingsMargin is the gap a dragged settings dialog keeps from the window's
// edges.
const settingsMargin = float32(12)

func (w *window) settingsDialog(c *ui.Context) {
	width, height := c.Size()
	pal := paletteFor(c)
	wash := uitokens.WashFor(w.store.Get().Background)
	panelW := min(820, width-64)
	panelH := min(620, height-64)
	// The offset the pointer drags the dialog by is from the centre, where it
	// starts: the bounds are the room that keeps its whole face inside the
	// window, a margin from the edges.
	dragBounds := ui.Rect{
		X: settingsMargin - (width-panelW)/2,
		Y: settingsMargin - (height-panelH)/2,
		W: max(0, width-panelW-2*settingsMargin),
		H: max(0, height-panelH-2*settingsMargin),
	}
	// A floating settings window over the shells, not a sheet on the window's
	// wash: the wash covered the shells, so the appearance settings could not be
	// tried against what they change. No scrim either: the dialog is a window of
	// its own, moved by its header, and the page stays in sight behind it.
	ui.DialogBase(c, &w.settingsOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.Transparent)
		panel.Width(panelW).Height(panelH).
			Radius(uitokens.CardRadius).Clip()
		floating(panel, pal)
		panel.LinearGradient(wash.Base(pal.Dark)).
			Draggable(&w.settingsDrag, ui.DraggableOptions{Bounds: &dragBounds, Rigid: true}).
			Children(func() {
				if w.settingsContent != nil {
					w.settingsContent.view(c)
				}
			})
	})
}

// reachDialog is the confirmation a person gets the first time they pick a
// machine this window cannot reach: where the credential comes from, what it is
// good for, and that the relay is the one checking.
func (w *window) reachDialog(c *ui.Context) {
	if w.reachPrompt == "" || w.fleet == nil {
		return
	}
	t := c.Theme()
	name := w.machineName(w.reachPrompt)
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Gap(10).Padding(18).Width(430).Children(func() {
			ui.Text(c, locales.ReachTitle(name)).FontSize(14).FontWeight(600)
			ui.Text(c, locales.ReachBody()).
				FontSize(11).TextColor(t.TextMuted)
			if w.reachErr != "" {
				ui.Text(c, locales.ReachFailed(w.reachErr)).FontSize(11).TextColor(t.Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, locales.ActionCancel()).Clicked() {
					w.reachPrompt, w.reachErr = "", ""
					w.machineSel = machineLocal
				}
				if ui.PrimaryButton(c, locales.ReachConfirm()).Clicked() {
					w.askForReach()
				}
			})
		})
	})
	if !open {
		w.reachPrompt, w.reachErr = "", ""
		w.machineSel = machineLocal
	}
}

// currentTermSize returns the size the active terminal view fits, in columns
// and rows, or 80x24 when no terminal is available. It is the view's size rather
// than the screen's: a shell this window starts here is driven by this window,
// so it takes the size of the pane it is about to be drawn in — not the size of
// the screen another machine's window gave the shell that was on screen.
func (w *window) currentTermSize() (cols, rows int) {
	if tb := w.activeTab(); tb != nil {
		if c, r := tb.viewSize(); c > 0 && r > 0 {
			return c, r
		}
	}
	for _, tb := range w.tabs {
		if tb != nil {
			if c, r := tb.viewSize(); c > 0 && r > 0 {
				return c, r
			}
		}
	}
	return 80, 24
}

// viewSize is the size the pane this tab is drawn in last asked the shell for,
// and the screen's own size before a frame has drawn it.
func (t *tab) viewSize() (cols, rows int) {
	if c, r := t.conn.viewSize(); c > 0 && r > 0 {
		return c, r
	}
	if t.term != nil {
		return t.term.Size()
	}
	return 0, 0
}

// newTab asks the machine the panels are showing for a shell and gives it a tab
// to land in. On this machine that is the hub the window is already a client of;
// on another it is a create over the relay. It runs on the window's own thread —
// the tab is window state, and a goroutine that opened one would race the frame
// that draws it — and the hub's answer arrives as a `created` frame that adopts
// the tab waiting for it.
func (w *window) newTab() {
	machine := w.selectedMachine()
	var tb *tab
	if machine != "" && machine != machineLocal {
		tb = w.startTabOn(machine)
	} else {
		tb = w.startLocalTab()
	}
	if tb != nil {
		w.showInFocus(tb)
	}
}

// startLocalTab asks this machine's hub for a shell and gives it a tab to land
// in. It runs on the window's own thread — the tab is window state, and a
// goroutine that opened one would race the frame that draws it — and the hub's
// answer arrives as a `created` frame that adopts the tab waiting for it.
//
// The shell opens where the shell on screen stands (see followOn), falling back
// to the configured directory and then to the home directory, so a second tab
// continues where the first one is rather than at the top of the house.
func (w *window) startLocalTab() *tab {
	// What the new shell opens beside is read before the tab is opened: opening
	// one makes it the shell on screen, and the shell to follow is the one that
	// was on screen until now.
	from, dir := w.followOn(machineLocal)
	if dir == "" {
		dir = w.cwd
	}
	tb := w.openTab(machineLocal, "")
	if tb == nil {
		w.linkNote = w.noLinkReason(machineLocal)
		return nil
	}
	cols, rows := w.currentTermSize()
	w.local.create(cols, rows, dir, w.shell, from)
	return tb
}

// startTabOn asks another machine's hub for a shell there and gives it a tab
// here, the same act as New Tab on a machine the window reaches through the
// relay. The far hub names the shell, its `created` frame binds the waiting
// tab, and the tab that was opened for it is the tab watching it.
//
// Nothing about the shell is guessed here beyond the directory to open in: an
// empty cwd and shell mean the far machine's own defaults, which is what a
// person sitting at it would get.
func (w *window) startTabOn(machine string) *tab {
	// A shell asked for on a machine that is not there would be a tab with
	// nothing in it and a create frame queued for a stream that never routes.
	// The roster already knows; say so instead of opening the tab.
	if !w.machineOnline(machine) {
		w.linkNote = locales.AppMachineOffline(w.machineName(machine))
		w.setNotice(w.linkNote)
		w.invalidate()
		diag.Infof("term", "shell not created: machine offline machine=%s", machine)
		return nil
	}
	// Read before the tab is opened, for the same reason as the local one: the
	// shell to follow is the one on screen until the new tab takes its place.
	from, dir := w.followOn(machine)
	tb := w.openTab(machine, "")
	if tb == nil {
		w.linkNote = w.noLinkReason(machine)
		return nil
	}
	w.linkNote = ""
	// The link may still be waiting for the relay to route it; the frame waits
	// with it rather than being dropped, which is what the panel's click would
	// otherwise do on the first use of a machine.
	cols, rows := w.currentTermSize()
	w.linkFor(machine).create(cols, rows, dir, "", from)
	return tb
}

// followOn is what a new shell on machine opens beside: the session to follow
// and the directory to fall back to when that session cannot be asked where it
// stands — a machine that does not answer, or a session on its way out. The far
// side answers with the directory the followed shell stands in now, so the new
// one continues there. With nothing to follow — the first shell, or one on
// another machine — both are empty and the far side uses its own default.
func (w *window) followOn(machine string) (session, dir string) {
	tb := w.activeTab()
	if tb == nil || tb.id == "" {
		return "", ""
	}
	if tb.machine != machine && !(tb.machine == "" && machine == machineLocal) {
		return "", ""
	}
	return tb.id, tb.cwd
}

// showInFocus puts a tab on screen. A new shell is a tab of its own: it does
// not take the focused pane's place, which in another terminal is what a tab
// pick or ⌘T never does. Picking a tab that belongs to a workspace brings that
// workspace and its layout back.
func (w *window) showInFocus(tb *tab) {
	w.selectTab(tb)
}

// kill ends the shell behind a tab — this machine's or another one's, through
// the link to whichever machine it lives on. The tab itself stays: the exit that
// follows marks it, and its last output is what the tab is for at that point.
//
// Ending needs control, and a window that is only watching does not have it, so
// it claims first — stealing when another client holds the lease. Both frames can
// go out back to back because the close carries no epoch: the hub checks that
// this client is the controller, rather than an epoch a claim has just moved.
//
// The claim here is a means to an end, not the 接管 a person asks for: the tab is
// left alone either way, and closing it is the tab strip's own ×.
func (w *window) kill(tb *tab) {
	if tb == nil {
		return
	}
	w.killSessionOn(tb.machine, tb.id)
}

// killSessionOn ends a shell this window may only be watching: a row of the
// panel, with no tab behind it. It is the same act as kill, addressed by machine
// and session id rather than by a tab.
func (w *window) killSessionOn(machine, sessionID string) {
	if sessionID == "" {
		return
	}
	l := w.linkFor(machine)
	if l == nil {
		return
	}
	if tb := w.tabOf(machine, sessionID); tb != nil && tb.held() {
		l.closeSession(sessionID)
		return
	}
	// Not holding it, or no tab at all: claim first. The steal is decided by who
	// the hub last said holds the shell, which is the same answer the row shows.
	steal := false
	if tb := w.tabOf(machine, sessionID); tb != nil {
		steal = tb.holder() != ""
	} else {
		steal = w.remoteHolder(machine, sessionID) != ""
	}
	l.claim(sessionID, steal)
	l.closeSession(sessionID)
}

// remoteHolder is who the hub last said controls a shell on another machine.
func (w *window) remoteHolder(machine, sessionID string) string {
	for _, s := range w.remoteSessions[machine] {
		if s.SessionID == sessionID {
			return s.Controller
		}
	}
	return ""
}

// reconcile brings the tabs in line with the sessions the hub reports. It runs
// on the UI thread.
func (w *window) reconcile(live []protocol.Summary) {
	w.live = live
	present := make(map[string]bool, len(live))
	for _, sum := range live {
		present[sum.SessionID] = true
	}
	for id := range w.dismissed {
		if !strings.ContainsRune(id, '\x00') && !present[id] {
			delete(w.dismissed, id)
		}
	}
	// Keep the exit frame's final output, and never reconcile remote tabs
	// against this machine's roster.
	for key, tb := range w.tabs {
		if tb == nil || tb.machine != machineLocal || tb.id == "" || present[tb.id] || tb.alive || tb.done {
			continue
		}
		// A pane showing this tab goes with it, the way removeTab takes one:
		// the tree is settled below, once the tab set has stopped moving.
		if n := w.leafOf(tb); n != nil {
			w.collapseLeaf(n, false)
		}
		tb.close()
		delete(w.tabs, key)
		w.order = dropKey(w.order, key)
	}

	// A shell that appeared gets one. A shell that has *ended* does not, even if
	// a list still names it: a tab is opened to watch something, and the one
	// thing an ended shell needs no watching for. This is what used to put a
	// 已退出 tab back on screen a moment after it was closed — a tab nobody
	// could get rid of, because the next session list put it back.
	for _, s := range live {
		if !s.Alive {
			continue
		}
		if tb := w.tabOf(machineLocal, s.SessionID); tb != nil {
			if tb.program == "" && s.Shell != "" {
				tb.program = programName(s.Shell)
			}
			// The hub owns the name: a rename from anywhere — this window, a
			// browser, a phone — arrives here, and a cleared title arrives as
			// the empty string, which is the shell going back to its program.
			tb.setTitle(s.Title)
			// The directory the hub last saw this shell in, which is where a
			// shell opened beside it will open.
			tb.cwd = s.Cwd
			continue
		}
		if w.dismissed[s.SessionID] {
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
	// A shell that appeared — started from a browser, say — has a tab and the
	// strip now points at it; with panes on screen the strip follows the pane
	// instead.
	w.syncSplit()
}

// nextOrdinal finds the lowest unused 1-based index for tabs on the given machine.
func (w *window) nextOrdinal(machine string) int {
	used := make(map[int]bool)
	for _, tb := range w.tabs {
		if tb != nil && tb.machine == machine && tb.ordinal > 0 {
			used[tb.ordinal] = true
		}
	}
	for i := 1; ; i++ {
		if !used[i] {
			return i
		}
	}
}

// mount gives a session its native terminal. The terminal reads and writes the
// session through the window's link, so it draws the real shell — the same one
// a browser sees — and types into it under the same lease a browser answers to.
func (w *window) mount(s protocol.Summary) error {
	tb := w.openTab(machineLocal, s.SessionID)
	if s.Title != "" {
		tb.setTitle(s.Title)
	}
	if s.Shell != "" {
		tb.program = programName(s.Shell)
	}
	return nil
}

// openTab gives the window a new tab watching a session on machine, attaching
// to it over that machine's link. A session id of "" is one the window has just
// asked the hub to create: the tab waits for the hub to name it.
//
// It returns nil when there is no link to that machine — which is a thing the
// caller must say out loud, because attaching a remote session id to the local
// hub instead would open a tab that shows nothing and explains nothing.
func (w *window) openTab(machine, sessionID string) *tab {
	l := w.linkTo(machine)
	if l == nil {
		diag.Warnf("term", "no link to open a tab on machine=%s session=%s: %s", machine, sessionID, w.noLinkReason(machine))
		log.Printf("tfdesktop: no link to %s: %s", machine, w.noLinkReason(machine))
		return nil
	}
	key := w.nextKey
	w.nextKey++
	prog := ""
	if machine == machineLocal {
		prog = programName(w.shell)
		if prog == "" {
			prog = programName(os.Getenv("SHELL"))
		}
	}
	tb := &tab{
		id:      sessionID,
		machine: machine,
		program: prog,
		ordinal: w.nextOrdinal(machine),
	}
	w.tabs[key] = tb
	w.order = append(w.order, key)
	w.selected = len(w.order) - 1
	if machine != "" {
		w.machineSel = machine
	}
	// The line a blank pane is reported against, opened and closed in one file:
	// a tab on this machine, this session, at this size. Everything after it —
	// attach sent, attached, first bytes, or a dropped frame — belongs to it.
	diag.Infof("term", "tab open machine=%s session=%s wired=%t", machine, sessionID, l.online())
	w.watchTab(tb, l, machine, sessionID)
	return tb
}

// linkTo is the link that speaks to machine right now: this machine's own, or
// the window's relay link to another machine — opening that link when the
// window has none yet. It is nil when the machine cannot be reached at all,
// which is a thing a caller must say out loud rather than paper over with a
// tab that shows nothing and explains nothing.
func (w *window) linkTo(machine string) *link {
	if l := w.linkFor(machine); l != nil {
		return l
	}
	if remote := w.remoteLink(machine); remote != nil {
		return remote.link
	}
	return nil
}

// watchTab gives a tab its terminal and attaches it to sessionID over l. It is
// what opening a tab does, and what re-attaching a tab does when the link it
// was on has gone away: the hub replays the session's ring from byte 0, so a
// fresh terminal is the screen that replay is meant to paint — feeding it to
// the old one would lay the replay over whatever the tab last showed.
func (w *window) watchTab(tb *tab, l *link, machine, sessionID string) {
	// The terminal this tab had, if any, goes away with the conn under it: its
	// reader is parked on a link that is never coming back.
	if tb.conn != nil {
		_ = tb.conn.Close()
	}
	if tb.term != nil {
		_ = tb.term.Close()
	}
	sess := newSession(sessionID, l, nil)
	conn := newTermConn(sess)
	conn.machine = machine
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
		// The screen lays out at the size the shell has, which is not always the
		// size this pane is. A viewer is a window onto a screen that already
		// exists: the replay it is fed was written for that screen's width, with
		// the program's cursor moves and erases measured against it, so laying
		// the same bytes out at this pane's width instead would wrap the lines
		// the machine's window has whole and put every later redraw of the
		// program in the wrong place — the pushed-over screen reading as another
		// screen rather than as a narrower view of the same one. The conn hands
		// the screen its size: this pane's while the window drives the shell
		// (see termConn.Resize), the shell's own while it watches (see
		// tabHandlers.Attached and window.followSize).
		ResizeByConn: true,
	})
	if err != nil {
		// The pane would be a white rectangle with no shell in it and no
		// explanation, which is the exact failure this app must not produce
		// silently. The log names it and the window says it once; the tab stays,
		// because the engine may come up late and the attach is still valid.
		diag.Errorf("term", "terminal view failed machine=%s session=%s: %v", machine, sessionID, err)
		log.Printf("tfdesktop: terminal: %v", err)
		w.setNotice(locales.AppTerminalUnavailable(err.Error()))
	}
	conn.watchScreen(term)
	tb.term, tb.conn, tb.sess = term, conn, sess
	tb.machine = machine
	tb.setLease("", "", 0)
	tb.takeLetGo()

	if sessionID != "" {
		sess.bind(sessionID)
		diag.Infof("term", "terminal ready machine=%s session=%s view=%t size=%s",
			machine, sessionID, term != nil, tabSize(term))
		l.attach(sessionID, 0)
	} else {
		// The hub's answer names this session; until it arrives the link holds
		// the tab's place.
		l.watch(sess)
	}
}

// tabSize is a terminal view's own size for a log line, or "unlaid" before the
// first frame has given it one. A view that is 0x0 when its replay arrives
// draws nothing and then never repaints, so the number matters.
func tabSize(term *terminal.Terminal) string {
	if term == nil {
		return "none"
	}
	cols, rows := term.Size()
	if cols <= 0 || rows <= 0 {
		return "unlaid"
	}
	return fmt.Sprintf("%dx%d", cols, rows)
}

// followServerSize lays a viewer's screen out at the size the machine reports
// for the shell's terminal.
//
// A shell has one screen, and the window that drives it decides its size: the
// program writes for that width, moves its cursor against it, and erases to the
// edges of it. Every other window watches that same screen, so its pane is a
// window onto it rather than a terminal of its own — a pane 60 columns wide
// showing a screen written for 120 shows the left 60 columns of it, exactly as
// the machine's own window does, and a program that redraws itself lands where
// it meant to. Laying the same bytes out at the pane's width instead is what
// made a pushed-over screen read as a different one: lines wrapped here and not
// there, and every redraw after the first landed a row or a column off.
//
// The controller is left alone: its own size is what it has just told the hub,
// and the screen took it in termConn.Resize.
func (w *window) followServerSize(tb *tab, sum protocol.Summary) {
	if tb == nil || tb.term == nil || tb.held() {
		return
	}
	if sum.Cols <= 0 || sum.Rows <= 0 {
		return
	}
	if tb.conn.layOut(sum.Cols, sum.Rows) {
		diag.Debugf("term", "viewer screen follows machine=%s session=%s size=%dx%d",
			tb.machine, tb.id, sum.Cols, sum.Rows)
	}
}

// followSizes is followServerSize over a machine's list, which is where a shell
// another window has resized becomes visible to this one.
func (w *window) followSizes(machine string, list []protocol.Summary) {
	for i := range list {
		w.followServerSize(w.tabOf(machine, list[i].SessionID), list[i])
	}
}

// reattachMachineTabs gives the tabs watching machine a link again after the
// one they were on ended. A tab on a dead link renders nothing new, ever: the
// click that finds it only selects it, so a machine whose relay stream dropped
// — a relay restart, a member flap, a slow viewer evicted under load — would
// leave every tab on it a blank pane with a live shell behind it. The machine's
// link as it now stands is l; every tab still on the old one is re-watched on
// it, and the hub's replay from byte 0 paints the shell's screen again.
//
// A tab whose shell has ended keeps its final screen: there is no session left
// to attach to, and wiping the last words of a dead shell to show a blank pane
// is a worse answer than the pane it already has.
func (w *window) reattachMachineTabs(machine string, l *link) {
	for _, key := range w.order {
		tb := w.tabs[key]
		if tb == nil || tb.machine != machine || tb.id == "" {
			continue
		}
		if tb.isEnded() {
			continue
		}
		if tb.sess == nil || tb.sess.link == l || !tb.sess.link.dead() {
			continue
		}
		diag.Infof("term", "re-attach machine=%s session=%s reason=link ended", machine, tb.id)
		w.watchTab(tb, l, machine, tb.id)
	}
}

// closeTab is what every user-facing close does: the view goes and the session
// is ended. A terminal tab and the shell behind it are one thing to the person
// closing it, and a close that only stopped watching left a shell running that
// they could not see — the one behaviour that made this client feel unlike a
// terminal. Ending needs the lease, so kill claims it first, as 结束 always did.
//
// A tab whose shell has already ended, or one still waiting for its session id,
// has nothing to end: it is only the view that goes.
func (w *window) closeTab(tb *tab) {
	if tb == nil {
		return
	}
	if tb.id != "" && tb.alive {
		w.kill(tb)
	}
	w.removeTab(tb)
}

// removeTab takes a tab away without touching its shell. It is the cleanup path
// — a machine that went away, a session the hub no longer lists, a tab that was
// already closed — not the user's close: closeTab ends the session first.
// Nothing is sent here; the hub forgets a watcher when its session goes away,
// and a detach would broadcast a session list that still holds a shell that is
// on its way out.
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
	// A pane showing this tab goes with it, quietly: the tree is settled after
	// the tab set has moved, because selecting the pane that is left would name
	// a strip index that is about to change. A workspace that is put away is
	// edited in place, so a tab closed from the sidebar takes its pane out of a
	// layout nobody is looking at too.
	w.detachTab(tb)
	if tb.id != "" && tb.machine == machineLocal {
		if w.dismissed == nil {
			w.dismissed = make(map[string]bool)
		}
		w.dismissed[tb.id] = true
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
	w.syncSplit()
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

// activeTab is the tab on screen, or nil with none open. With a split open
// that is the focused pane's shell: the composer types into it, the header and
// the status bar describe it, and a new shell opens beside it.
func (w *window) activeTab() *tab {
	if w.split != nil && w.focus != nil && w.focus.tab != nil {
		return w.focus.tab
	}
	if w.selected < 0 || w.selected >= len(w.order) {
		return nil
	}
	return w.tabs[w.order[w.selected]]
}

// localHeldByOthers counts the shells on this machine that another client is
// driving. It is the machine panel's answer to "why is my typing going nowhere?"
// for a person who never saw the glow, and it counts a shell this window holds
// as nobody's but this window's — whichever tab is holding it.
func (w *window) localHeldByOthers() int {
	n := 0
	for _, s := range w.live {
		if s.Controller == "" {
			continue
		}
		if tb := w.tabOf(machineLocal, s.SessionID); tb != nil && tb.held() {
			continue
		}
		n++
	}
	return n
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

// tab is the tab watching this handler's session. It is matched by the session
// object, not by its id: session ids are counted per machine ("s1" on every
// machine), and a window driving another machine at the same time has a local
// "s1" and a remote one. An id lookup would hand one shell's role frame, exit or
// close to the other shell's tab.
func (h tabHandlers) tab() *tab { return h.w.tabOfSession(h.sess) }

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
		if tb.program == "" && sum.Shell != "" {
			tb.program = programName(sum.Shell)
		}
		w.followServerSize(tb, sum)
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
			if tb.held() {
				// The view's size, which syncSize reads off the conn: the
				// screen may still be the size the shell had a moment ago.
				h.conn.syncSize(0, 0)
			}
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
		tb := h.tab()
		if tb == nil {
			return
		}
		held := tb.held()
		if tb.takeLetGo() {
			// The window let this go a frame ago: the role frame answering it is
			// not a machine taking the shell back.
			held = false
		}
		tb.setLease(role, controller, epoch)
		if tb.held() {
			// Taking the lease re-establishes the shell's size, whatever this
			// conn last told the hub: the window that held it until now may
			// have resized the shell, and the shell is this pane's size from
			// here on.
			h.conn.takeSize()
		}
		switch {
		case !held || tb.held():
			// Nothing changed hands: a viewer is still a viewer, or the lease is
			// still ours.
		default:
			// A shell this window was driving has been taken back. The window
			// keeps watching it and keeps its link to the machine: the pane says
			// 只读, the popover explains who is typing, and the button in it takes
			// the shell back.
			//
			// This used to be split — a shell of this machine's kept its link and
			// opened the popover, while a shell on another machine's dropped the
			// whole connection and removed the tab. That is the opposite of what
			// the difference between them calls for: a remote shell is the one a
			// person reached out to, so it is the one worth keeping, and tearing
			// the relay stream down meant the 接管 button was never drawn at all.
			w.leaseOpen = true
		}
		w.invalidate()
	})
}

func (h tabHandlers) Exit(code int) {
	w := h.w
	// The line goes straight to the terminal: the hub has already flushed the
	// shell's last output, and this is the window's own word on the end.
	// The marker is one message in both languages, escape bytes and all: the
	// catalog carries the whole line the terminal is fed.
	h.conn.feed([]byte(locales.TermExitMarker(code)), false)
	w.update(func() {
		if tb := h.tab(); tb != nil {
			tb.finish(code)
		}
		w.invalidate()
	})
}

// Closed is the hub's old acknowledgement that this client asked to end a
// session. It no longer arrives: a close produces the same exit frame for every
// watcher, including whoever asked, so the tab stays and shows its last output.
// An older daemon still sends it, and removing the tab on it would take that
// output away, so it is read as the exit it stands for.
func (h tabHandlers) Closed() {
	h.Exit(0)
}

func (h tabHandlers) Gone(reason string) {
	w := h.w
	w.update(func() {
		if tb := h.tab(); tb != nil && tb.id == "" {
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
	w := h.w
	switch code {
	case protocol.ErrNotController:
		w.update(func() {
			// The refusal is about one shell while the popover is one window-wide
			// panel, so it opens only for the shell on screen: a background tab's
			// refused keystroke must not put a panel over a shell that is fine.
			if tb := h.tab(); tb != nil && tb == w.activeTab() {
				w.leaseOpen = true
			}
			w.invalidate()
		})
	case protocol.ErrControlOff:
		// The machine said no: it allows other clients to watch and not to type.
		// There is no holder to name, so this is not the popover — it is the line
		// under the machines, which is where a refusal that belongs to a whole
		// machine is explained.
		w.update(func() {
			w.linkNote = locales.AppControlOff()
			// A create that was refused leaves the tab that was waiting for a shell
			// with nothing to wait for, which is a tab nobody can do anything with.
			if tb := h.tab(); tb != nil && tb.id == "" {
				w.removeTab(tb)
			}
			w.invalidate()
		})
	}
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

	// alive is whether the shell is still running: false until the attach that
	// opens the tab says otherwise, and false again the moment the exit frame
	// arrives — see finish, which is the only place it is turned off and the
	// reason it is not left to the session list.
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

	// letGo is set while the window is giving the lease up on purpose, so the
	// role frame that answers its own release is not mistaken for a machine
	// taking the shell back — which would end the window's connection to it.
	letGo bool

	// sent is what this window has typed into the shell, newest last. The
	// history bar above the composer recalls it, the way the browser client's
	// does; it is this window's own record, not the shell's, because a shell
	// reached through the relay does not hand its history over.
	sent []string

	// cwd is the directory the hub last reported for this shell. It is where a
	// shell opened beside this one falls back to when the machine cannot say
	// where its processes stand, and what the tab's own inspection shows.
	cwd string

	// ws is the workspace whose layout holds this shell, or nil for a tab of its
	// own. A split's panes share one; switching to another tab puts the workspace
	// away rather than rearranging it.
	ws *workspace
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
// The popover is drawn from this answer, and the glow is drawn from it for a
// shell of this machine's — see drivenElsewhere.
// takeLetGo reports whether the window is waiting for the lease's release to come
// back, and stops waiting for it.
func (t *tab) takeLetGo() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	was := t.letGo
	t.letGo = false
	return was
}

func (t *tab) held() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.role == protocol.RoleController
}

// holder is who holds the lease when this window does not. It is "" both when
// this window holds the lease and when nobody does; ask known and held to tell
// those apart.
func (t *tab) holder() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.role == protocol.RoleController {
		return ""
	}
	return t.controller
}

// known reports whether the window has been told which side of the lease it is
// on. It has not until the first role frame arrives, and that is a third state
// rather than a quiet yes or no: a window that read the silence as 只读 would
// mark every shell a person had just opened, for as long as the hub took to
// answer.
func (t *tab) known() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.role != ""
}

// letGoLease gives up the lease of a shell this window is no longer showing. The
// hub hands an unclaimed shell to the machine it runs on, which is how that
// machine's window gets its keyboard back.
func (t *tab) letGoLease() {
	if t.conn == nil {
		return
	}
	t.mu.Lock()
	if t.letGo {
		t.mu.Unlock()
		return
	}
	t.letGo = true
	t.mu.Unlock()
	t.conn.releasing()
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

// finish records that the shell has ended, and it is the only place the tab
// learns it. The exit frame is the hub's own word on it, and a session list
// never gives that word: the hub forgets a session the moment its process exits,
// so the list that follows an exit does not name the session at all — there is
// no summary left to say alive=false with. A tab that waited for one waited
// forever, which left the window drawing a live shell's controls over a shell
// that was gone: 结束 on a session the hub would only answer no_session to, no
// 关闭 on the row, no dimmed pill, no 已退出 banner.
func (t *tab) finish(code int) {
	t.mu.Lock()
	t.done = true
	t.exit = code
	t.mu.Unlock()
	// Read and written on the UI thread only, like every other use of the field.
	t.alive = false
}

// size is the terminal's own columns and rows, as "80x24". It is the one number
// that tells two shells started in the same directory apart, so it is what the
// panel's row shows — the same row the browser client draws.
func (t *tab) size() string {
	t.mu.Lock()
	term := t.term
	t.mu.Unlock()
	if term == nil {
		return ""
	}
	cols, rows := term.Size()
	return fmt.Sprintf("%dx%d", cols, rows)
}

// name is what the shell is called: its title once the shell has named itself,
// and what it is until then. It carries no state. The panel's row says 已退出 in
// a badge beside it, the pane says it in a corner, the status bar says it after
// the size — and the tab strip says it with the dot and the dimming, which is
// all a 30-high pill has room for: a name with a state word appended is a name
// the strip cuts in half to show the state.
func (t *tab) name() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.nameLocked()
}

func (t *tab) nameLocked() string {
	name := t.title
	if name == "" {
		// Nothing named this shell yet. Say what the tab is instead of
		// showing a session id, which tells the reader nothing.
		ordinal := t.ordinal
		if ordinal <= 0 && strings.HasPrefix(t.id, "s") {
			if n, err := strconv.Atoi(t.id[1:]); err == nil && n > 0 {
				ordinal = n
			}
		}
		switch {
		case t.program != "" && ordinal > 0:
			name = locales.TabProgramOrdinal(t.program, ordinal)
		case t.program != "":
			name = t.program
		case ordinal > 0:
			name = locales.TabOrdinal(ordinal)
		case strings.HasPrefix(t.id, "s"):
			name = locales.TabById(t.id[1:])
		case t.id != "":
			name = locales.TabById(t.id)
		default:
			name = locales.TabFallback()
		}
	}
	return name
}

// exitLine is what the tooltip says about a shell that has ended: the code it
// ended with, which is the one thing the strip has no room for and the one
// thing worth knowing when it is not zero.
func (t *tab) exitLine() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.done {
		return ""
	}
	return locales.TabExitedCode(t.exit)
}

// isPending reports whether this tab is still waiting for the hub to assign a session id.
func (t *tab) isPending() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.id == ""
}

// isEnded reports whether this tab has finished/exited.
func (t *tab) isEnded() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.done || (t.id != "" && !t.alive)
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

// close releases the terminal and detaches the view. It does not end the
// session: the user-facing closeTab sends the end first, and the cleanup paths
// (a lost machine, a session the roster dropped) need the view gone without
// touching a shell somebody else may still be watching.
func (t *tab) close() {
	if t.term != nil {
		_ = t.term.Close()
	}
	if t.conn != nil {
		_ = t.conn.Close()
	}
	// The link keeps a session object per id, and a closed tab's must not stay
	// there: the next tab on the same shell would register itself behind a
	// closed conn, and the replay would be written to a terminal nobody draws.
	// See (*link).watch, which takes over the id for the same reason.
	if t.sess != nil {
		t.sess.link.unwatchSession(t.sess)
	}
}

// serviceLabel is the line the header, the status bar and the rail's tooltip
// carry: where this machine's browser client can be reached, and what the relay
// is doing. The relay belongs here rather than in the machines' panel — it is a
// pipe, not a machine — and this is the line that says whether the fleet this
// window lists is reachable at all.
func (w *window) serviceLabel() string {
	parts := []string{}
	if url := w.fleet.BaseURL(); url != "" {
		parts = append(parts, url)
	} else {
		parts = append(parts, locales.ServiceOff())
	}
	if w.relayOn() {
		switch st := w.fleet.RelayState(); {
		case st.Connected:
			parts = append(parts, locales.RelayConnected())
		case st.Connecting:
			parts = append(parts, locales.RelayConnecting())
		default:
			parts = append(parts, locales.RelayDown())
		}
	}
	return strings.Join(parts, " · ")
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

// Relay changes also repaint the settings dialog.
func (w *window) invalidateSettings() { w.invalidate() }

func (w *window) openSettings() {
	if w.settingsContent == nil {
		w.settingsContent = newSettingsWindow(w, w.store, w.fleet)
	}
	w.settingsOpen = true
	w.invalidate()
}

// openSettingsAt opens the settings window on a page. The menu's 诊断日志… uses
// it, so the log is one item away rather than a page a person has to remember is
// there.
func (w *window) openSettingsAt(pane int) {
	w.openSettings()
	if w.settingsContent != nil {
		w.settingsContent.pane = pane
	}
	w.invalidate()
}
