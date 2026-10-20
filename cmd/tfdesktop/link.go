package main

// The window's end of the session protocol.
//
// Every shell the window shows is reached the way a browser reaches one: the
// window is a client of a hub. This machine's hub is spoken to over an
// in-process pipe (hub.Pipe); another machine's is spoken to through the relay
// (internal/relayclient). Both carry identical frames, so the arbitration of
// who may type happens in exactly one place — the hub that owns the PTY — and
// the window is subject to it like any other client.
//
// What that buys: a shell a browser has taken over reads as taken over here,
// with the lease's epoch on every frame the window sends, and the window takes
// its own machine's shell back without asking anyone.

import (
	"encoding/json"
	"sync"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
)

// Handlers is what a link reports about one session. They are called from the
// link's reader goroutine; a handler that touches window state must go through
// Window.Update, as the watch loop's does.
type Handlers interface {
	// Attached is the session as the hub sees it, with the cursor the replay
	// ended at.
	Attached(sum protocol.Summary, cursor int64, truncated bool)
	// Created is the hub naming a session this side asked for.
	Created(id string)
	// Output is bytes to show. Resync means the screen was reset first.
	Output(data []byte, resync bool)
	// Role is the lease as it now stands: this window's role, who holds it, and
	// the epoch input must carry.
	Role(role, controller string, epoch uint64)
	// Exit is the process ending; Closed is the session going away; Gone is the
	// link itself going away, with the reason.
	Exit(code int)
	Closed()
	Gone(reason string)
	// Failed is an error frame about this session.
	Failed(code, message string)
}

// link is one conversation with one hub. A machine's link is shared by every
// session the window watches on that machine: the protocol multiplexes by
// session id, exactly as it does over a WebSocket.
type link struct {
	// send puts one frame on the wire. It is set when the link opens and
	// cleared when it goes away, so a session typed into during a gap is
	// refused here rather than half-sent. The window's thread sets it and the
	// link's reader goroutine clears it, so it is reached through setSend and
	// sendMu rather than written directly.
	send   func(m protocol.Msg) bool
	sendMu sync.Mutex

	// onSessions is the hub's session list, pushed on every change. onGone is
	// the link itself ending, with the reason. onOpen is the link's wire coming
	// up — for a relay stream, the relay routing it — which is when a link that
	// replaces a dead one can pick the orphaned tabs back up. All three are set
	// by the link's owner; none is required.
	onSessions func([]protocol.Summary)
	onGone     func(reason string)
	onOpen     func()

	mu       sync.Mutex
	sessions map[string]*session
	// pending are the sessions the window has asked the hub for and the hub has
	// not named yet: a create is in flight, and its answer adopts the oldest.
	pending []*session
	closed  bool
}

func newLink() *link {
	return &link{sessions: map[string]*session{}}
}

// dead reports whether this link has ended. A tab watching a session on a dead
// link can never render again — nothing re-attaches it on its own — so the
// window re-watches such a tab on the machine's link as it now stands.
func (l *link) dead() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

// session is the window's view of one shell on the far side of a link: what it
// is watching, and the lease it holds — or does not.
type session struct {
	id       string
	link     *link
	handlers Handlers

	// wake is signalled when output arrives or the session ends, so a terminal
	// blocked in Read looks again.
	wake chan struct{}

	mu         sync.Mutex
	role       string
	controller string
	epoch      uint64
	// ended is what the exit frame leaves behind: see holds.
	ended bool
}

func newSession(id string, l *link, h Handlers) *session {
	return &session{id: id, link: l, handlers: h, wake: make(chan struct{}, 1)}
}

// ---- frames out ----------------------------------------------------------

func (l *link) attach(id string, cursor int64) {
	l.sendMsg(protocol.Msg{T: protocol.CTAttach, SessionID: id, Cursor: cursor})
}

func (l *link) detach(id string) {
	l.sendMsg(protocol.Msg{T: protocol.CTDetach, SessionID: id})
}

// claim asks for the lease. Steal takes it from a live holder; without it the
// hub refuses and names the holder.
func (l *link) claim(id string, steal bool) {
	l.sendMsg(protocol.Msg{T: protocol.CTClaim, SessionID: id, Steal: steal})
}

func (l *link) release(id string) {
	l.sendMsg(protocol.Msg{T: protocol.CTRelease, SessionID: id})
}

// closeSession ends the session. The epoch is not sent: it only guards against a
// frame from a holder who has since been replaced, and the hub checks that this
// client is the controller anyway. Sending the epoch this link happens to
// remember makes the close fail in the case it is most needed — right after a
// claim, before the role frame that moves the epoch has landed.
func (l *link) closeSession(id string) {
	l.sendMsg(protocol.Msg{T: protocol.CTClose, SessionID: id})
}

// input types into a session, at the epoch this window last saw. A window that
// does not hold the lease is refused by the hub, so the frame is sent
// regardless and answered there: one place decides, for every client.
func (l *link) input(id string, data []byte) {
	l.sendMsg(protocol.Msg{T: protocol.CTInput, SessionID: id, Data: data, Epoch: l.epochOf(id)})
}

func (l *link) resize(id string, cols, rows int) {
	if l == nil || id == "" {
		return
	}
	l.sendMsg(protocol.Msg{T: protocol.CTResize, SessionID: id, Cols: cols, Rows: rows, Epoch: l.epochOf(id)})
}

// create asks the hub for a new shell and attaches this window to it, so the
// window holds the lease of what it made, as a browser that creates one does.
// from names a live session of that machine's whose working directory the new
// shell opens in; see protocol.Msg.From.
func (l *link) create(cols, rows int, cwd, shell, from string) {
	l.sendMsg(protocol.Msg{
		T: protocol.CTCreate, Cols: cols, Rows: rows, Cwd: cwd, Shell: shell, From: from,
	})
}

// rename names a session: the title every client's tab and row reads for it.
// The lease is not this frame's business — a title is a label on a shell
// everybody watching sees, not the keyboard — so it is sent whatever role this
// window holds, and the hub's answer is the session list everybody gets.
func (l *link) rename(id, title string) {
	if l == nil || id == "" {
		return
	}
	l.sendMsg(protocol.Msg{T: protocol.CTRename, SessionID: id, Title: title})
}

func (l *link) list() {
	l.sendMsg(protocol.Msg{T: protocol.CTList})
}

func (l *link) epochOf(id string) uint64 {
	if s := l.session(id); s != nil {
		return s.epochOf()
	}
	return 0
}

func (l *link) sendMsg(m protocol.Msg) {
	l.sendMu.Lock()
	send := l.send
	l.sendMu.Unlock()
	if send == nil {
		return
	}
	send(m)
}

// setSend installs the link's writer, or takes it away with nil.
func (l *link) setSend(send func(m protocol.Msg) bool) {
	l.sendMu.Lock()
	l.send = send
	l.sendMu.Unlock()
}

// ---- frames in -----------------------------------------------------------

// dispatch hands one decoded frame to the session it names, or to the link
// itself when it names none.
func (l *link) dispatch(m protocol.Msg) {
	l.mu.Lock()
	closed := l.closed
	l.mu.Unlock()
	if closed {
		return
	}
	switch m.T {
	case protocol.STHello, protocol.STSessions:
		if m.Sessions != nil && l.onSessions != nil {
			l.onSessions(m.Sessions)
		}
		return
	case protocol.STCreated:
		l.adopt(m.SessionID)
		return
	}
	s := l.session(m.SessionID)
	if s == nil {
		return
	}
	switch m.T {
	case protocol.STAttached:
		if m.Session != nil {
			s.handlers.Attached(*m.Session, m.Cursor, m.Truncated)
		}
	case protocol.STOutput:
		s.handlers.Output(m.Data, m.Resync)
	case protocol.STRole:
		if m.Role != "" {
			s.setRole(m.Role, m.Controller, m.Epoch)
			s.handlers.Role(m.Role, m.Controller, m.Epoch)
		}
	case protocol.STExit:
		s.end()
		s.handlers.Exit(m.ExitCode)
	case protocol.STClosed:
		s.handlers.Closed()
	case protocol.STError:
		s.handlers.Failed(m.Code, m.Message)
	}
}

// adopt names a session the window is waiting for: the hub has created it, so
// the tab waiting for it becomes the tab watching it, and the session list that
// follows finds it already there.
func (l *link) adopt(id string) {
	l.mu.Lock()
	var s *session
	if len(l.pending) > 0 {
		s = l.pending[0]
		l.pending = l.pending[1:]
	}
	l.mu.Unlock()
	if s == nil || id == "" {
		return
	}
	s.bind(id)
	if s.handlers != nil {
		s.handlers.Created(id)
	}
}

// session is the link's session state, or nil if this link is not watching it.
func (l *link) session(id string) *session {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sessions[id]
}

// watch registers a session with the link, so frames for it have somewhere to
// go. A second watch of the same id keeps the first.
func (l *link) watch(s *session) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if s.id == "" {
		l.pending = append(l.pending, s)
		return
	}
	if _, ok := l.sessions[s.id]; !ok {
		l.sessions[s.id] = s
	}
}

// unwatch stops routing frames for a session to it.
func (l *link) unwatch(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.sessions, id)
}

// close ends the link. Every session on it is told, as a dropped connection
// tells them.
func (l *link) close(reason string) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	l.closed = true
	sessions := make([]*session, 0, len(l.sessions))
	for _, s := range l.sessions {
		sessions = append(sessions, s)
	}
	l.sessions = map[string]*session{}
	l.mu.Unlock()

	l.setSend(nil)
	if l.onGone != nil {
		l.onGone(reason)
	}
	for _, s := range sessions {
		s.handlers.Gone(reason)
	}
}

// ---- session state -------------------------------------------------------

func (s *session) epochOf() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.epoch
}

// bind gives the session its id, once the hub has named it. Until then the
// window has asked for a shell and not yet been told which one it got.
func (s *session) bind(id string) {
	s.mu.Lock()
	s.id = id
	s.mu.Unlock()
	if s.link == nil {
		return
	}
	l := s.link
	l.mu.Lock()
	if _, ok := l.sessions[id]; !ok {
		l.sessions[id] = s
	}
	l.mu.Unlock()
}

func (s *session) setRole(role, controller string, epoch uint64) {
	s.mu.Lock()
	s.role, s.controller, s.epoch = role, controller, epoch
	s.mu.Unlock()
}

// holds is what a terminal asks before it sends anything: this window holds the
// lease, and there is still a shell to send to. A shell that has ended takes no
// typing — the pane keeps its last output on screen, and keys over it would
// echo in the view and be answered no_session — and its size is no longer
// anything's business.
func (s *session) holds() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.role == protocol.RoleController && !s.ended
}

// ended is set the moment the exit frame arrives, before the window is told:
// the wire stops taking frames in the same breath as the tab learning the shell
// is gone.
func (s *session) end() {
	s.mu.Lock()
	s.ended = true
	s.mu.Unlock()
}

// holder is who holds the lease when it is not this window.
func (s *session) holder() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.role == protocol.RoleController {
		return ""
	}
	return s.controller
}

// ---- codec ---------------------------------------------------------------

// encode is one frame on the wire. Both transports carry JSON: the hub's pipe
// speaks the protocol directly, and the relay carries it base64'd inside a
// tunnel frame, which internal/relayclient unwraps before it gets here.
func encode(m protocol.Msg) ([]byte, error) { return json.Marshal(m) }

// decode is one frame off the wire.
func decode(b []byte) (protocol.Msg, error) {
	var m protocol.Msg
	err := json.Unmarshal(b, &m)
	return m, err
}
