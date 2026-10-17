package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

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

// noLinkReason is why the window has no link to a machine, in the words the
// panel and the log should use.
func (w *window) noLinkReason(machine string) string {
	if machine == machineLocal || machine == "" {
		return "本机链路未就绪"
	}
	if !w.relayOn() {
		return "未接入中继"
	}
	if w.fleet.RelayClientErr() != "" {
		return "客户端链路失败：" + w.fleet.RelayClientErr()
	}
	return "没有客户端令牌，无法连接其他机器"
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

// followLease is what the window does about the shell on screen, and about the
// ones that are not.
//
// A window drives what it is showing. Opening a tab on another machine takes
// that shell's lease (see openTab), and this keeps the rule true as the screen
// moves: a shell on another machine that leaves the screen is let go, so 切到本机
// does not leave the far machine behind 正在被控制 with a keyboard that no longer
// works — the hub hands a lease nobody holds to the machine the shell runs on,
// which is how that machine's window gets its keyboard back — and the shell that
// comes on screen is taken, so coming back to it drives it again without a click.
//
// A shell of this machine's that nobody holds is claimed from here too: a holder
// that lapsed, or a client that let go, must not leave this machine's own shell
// mute. What it never takes is a shell of this machine's from another client that
// holds it — that is 拿回控制权 in the pane, and it ends their session with this
// machine's shell, so it stays a person's click. Another machine's shell it does
// take, because reaching that machine is the consent, and the machine's own
// window keeps the way back there. The taking happens on the way in rather than
// every frame: two windows looking at the same shell would otherwise take it from
// each other for as long as both looked.
func (w *window) followLease() {
	on := w.activeTab()

	// A shell on another machine that this window is not showing is not one it
	// drives. Derived from the shells and the screen rather than remembered, so
	// no sequence of clicks can leave a shell held with nobody looking at it.
	for _, tb := range w.tabs {
		if tb == nil || tb == on || tb.machine == machineLocal || !tb.held() {
			continue
		}
		tb.letGoLease()
	}

	if on == nil || on.conn == nil || on == w.arranged {
		return
	}
	// The shell on screen has changed: settle its lease once. A claim the hub
	// refuses — a machine that allows no other client to control it — is answered
	// on screen, and is not retried every frame.
	w.arranged = on
	if on.held() {
		return
	}
	if on.machine == machineLocal && on.holder() != "" {
		// This machine's own shell, held by another client: taking it back ends
		// that client's session with this machine's shell, so it stays a person's
		// click — 拿回控制权 in the pane — and never a side effect of looking at it.
		return
	}
	// Another machine's shell on screen is one this window drives: reaching that
	// machine is the consent. The machine's own window is left read-only with the
	// way back — 拿回控制权, which the hub always honours — and letting this
	// happen on the way in rather than every frame is what keeps two windows
	// looking at the same shell from taking it from each other for as long as
	// both looked.
	on.conn.claiming(on.machine != machineLocal)
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
	w.followLease()

	// The appearance is a saved preference, and the ui reads the theme once per
	// frame, so every frame of every window asks for it.
	mode := w.store.Get().Theme
	applyTheme(c, mode)

	// A tab strip's own keys, because the panel's rows are not the only way
	// between shells: ⌘⇧] and ⌘⇧[ walk this window's tabs, wherever each shell
	// runs. The terminal takes the keys it binds; these it does not.
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketRight) {
		w.stepTab(1)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketLeft) {
		w.stepTab(-1)
	}

	ui.Row(c).Fill().Background(c.Theme().Background).Children(func() {
		w.activityRail(c, mode)
		w.sidebar(c, mode)
		ui.Column(c).Fill().Children(func() {
			w.header(c, labels)
			w.body(c, labels)
			w.statusBar(c)
		})
		w.reachDialog(c)
	})
}

// loseMachine is what happens when a machine takes its shells back from this
// window: the tabs it had open there are closed, the link to that machine is
// dropped, the panels return to this machine, and the window says who did it.
//
// A connection that can no longer type is not worth leaving on screen pretending
// otherwise, and the person at this keyboard needs to know why their shell
// vanished — the machine they were driving took it back.
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
	w.setNotice(name + " 拿回了控制权：与它的连接已断开")
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
			ui.Text(c, "接管 "+name).FontSize(14).FontWeight(600)
			ui.Text(c, "这个窗口会拿本机的设备密钥向中继申请一枚短时接管令牌，中继核验这台机器是否已注册。拿到之后，同一中继上的机器就都能在这里驱动了。").
				FontSize(11).TextColor(t.TextMuted)
			if w.reachErr != "" {
				ui.Text(c, "中继没有发下接管令牌："+w.reachErr).FontSize(11).TextColor(t.Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					w.reachPrompt, w.reachErr = "", ""
					w.machineSel = machineLocal
				}
				if ui.PrimaryButton(c, "申请并接管").Clicked() {
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

// newTab asks the machine the panels are showing for a shell and gives it a tab
// to land in. On this machine that is the hub the window is already a client of;
// on another it is a create over the relay. It runs on the window's own thread —
// the tab is window state, and a goroutine that opened one would race the frame
// that draws it — and the hub's answer arrives as a `created` frame that adopts
// the tab waiting for it.
func (w *window) newTab() {
	if machine := w.selectedMachine(); machine != machineLocal {
		w.newTabOn(machine)
		return
	}
	w.openTab(machineLocal, "")
	w.local.create(80, 24, w.cwd, w.shell)
}

// newTabOn asks another machine's hub for a shell there and gives it a tab here,
// the same act as New Tab on a machine the window reaches through the relay. The
// far hub names the shell, its `created` frame binds the waiting tab, and the
// tab that was opened for it is the tab watching it.
//
// Nothing about the shell is guessed here: an empty cwd and shell mean the far
// machine's own defaults, which is what a person sitting at it would get.
func (w *window) newTabOn(machine string) {
	tb := w.openTab(machine, "")
	if tb == nil {
		w.linkNote = w.noLinkReason(machine)
		return
	}
	w.linkNote = ""
	// The link may still be waiting for the relay to route it; the frame waits
	// with it rather than being dropped, which is what the panel's click would
	// otherwise do on the first use of a machine.
	w.linkFor(machine).create(80, 24, "", "")
}

// kill ends the shell behind a tab — this machine's or another one's, through
// the link to whichever machine it lives on. The tab itself goes when that hub
// reports the session gone. A shell the window does not hold is refused by that
// hub, the same as typing into it.
func (w *window) kill(tb *tab) {
	if tb == nil || tb.id == "" {
		return
	}
	if l := w.linkFor(tb.machine); l != nil {
		l.closeSession(tb.id)
	}
}

// reconcile brings the tabs in line with the sessions the hub reports. It runs
// on the UI thread.
func (w *window) reconcile(live []protocol.Summary) {
	w.live = live
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
//
// It returns nil when there is no link to that machine — which is a thing the
// caller must say out loud, because attaching a remote session id to the local
// hub instead would open a tab that shows nothing and explains nothing.
func (w *window) openTab(machine, sessionID string) *tab {
	l := w.linkFor(machine)
	if l == nil {
		remote := w.remoteLink(machine)
		if remote == nil {
			log.Printf("tfdesktop: no link to %s: %s", machine, w.noLinkReason(machine))
			return nil
		}
		l = remote.link
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
		if machine != machineLocal {
			// Reaching a machine is the consent — the relay handed this window a
			// control token for it — and a tab here is opened to drive what it
			// watches, so the lease comes with the tab. Without this the window
			// arrived as a spectator of a shell it had just asked to work in: the
			// machine's own window kept the lease, and this one sat read-only
			// behind a glow that said someone else was typing.
			//
			// The machine whose shell it is goes read-only and says so — the
			// glow, 正在被控制, and 拿回控制权, which the hub always honours for
			// the machine the shell runs on.
			l.claim(sessionID, true)
		}
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

// activeTab is the tab on screen, or nil with none open.
func (w *window) activeTab() *tab {
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
		switch {
		case !held || tb.held():
			// Nothing changed hands: a viewer is still a viewer, or the lease is
			// still ours.
		case tb.machine != "" && tb.machine != machineLocal:
			// A shell on another machine that this window was driving has been
			// taken back — by that machine's own window, which is the only client
			// that can do it without stealing. Per the machine's own rule, our
			// connection to it is over: see loseMachine.
			w.loseMachine(tb.machine)
		default:
			// A shell of this machine's, taken by someone else: it says so where
			// the typing used to go. The glow and the tab's mark say which shell,
			// and the popover is the only place that offers it back.
			w.leaseOpen = true
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
			w.linkNote = "这台机器不允许其他客户端控制它：在它的 设置 → 服务 → 允许被控制 里打开。"
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

	// letGo is set while the window is giving the lease up on purpose, so the
	// role frame that answers its own release is not mistaken for a machine
	// taking the shell back — which would end the window's connection to it.
	letGo bool
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

// holder is who holds the lease when this window does not.
func (t *tab) holder() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.role == protocol.RoleController {
		return ""
	}
	return t.controller
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
	if t.conn != nil {
		_ = t.conn.Close()
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
		parts = append(parts, "本机服务已关闭")
	}
	if w.relayOn() {
		switch st := w.fleet.RelayState(); {
		case st.Connected:
			parts = append(parts, "中继已连接")
		case st.Connecting:
			parts = append(parts, "中继连接中…")
		default:
			parts = append(parts, "中继断开")
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
