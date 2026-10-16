// Package hub multiplexes many PTY sessions onto one WebSocket per client and
// decides who is allowed to type.
//
// Two rules shape everything here:
//
//   - attach/detach (is this client watching?) is separate from create/close
//     (does the process exist?). A client that goes away never kills a shell.
//   - a session has at most one controller. Viewers get a read-only mirror.
//     A PTY has a single cols/rows pair, and letting every client resize it
//     makes the screen thrash; letting every client type makes a phone in a
//     pocket interfere with the desktop.
package hub

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/session"
)

const (
	// outboundQueue is how many frames may be in flight to one client before
	// it is considered too slow to keep.
	outboundQueue = 512
	writeTimeout  = 15 * time.Second
	pingInterval  = 30 * time.Second
	pingTimeout   = 10 * time.Second

	// DefaultIdleController is how long a controller may sit silent before it
	// loses control, so a phone left in a pocket cannot lock the desktop out.
	DefaultIdleController = 30 * time.Minute
)

// Options configures a Hub.
type Options struct {
	// Host is reported to clients in the handshake.
	Host protocol.Host
	// OriginPatterns widens the WebSocket Origin check beyond same-origin.
	// Only needed when a dev server on another port talks to the daemon.
	OriginPatterns []string
	// Token, when set, is required from every client.
	Token string
	// IdleTimeout overrides DefaultIdleController.
	IdleTimeout time.Duration
	// ControlOff starts the hub with every client but this machine's own window
	// refused a shell's lease. Joining a relay is the consent to be driven from
	// elsewhere, which is why the zero value allows it; this is the switch a
	// person flips when the shells on this machine should be theirs alone. It can
	// be changed while the hub runs through SetControlAllowed.
	ControlOff bool
}

// Hub owns the sessions on one machine and every connected client.
type Hub struct {
	mgr  *session.Manager
	opts Options
	host protocol.Host
	idle time.Duration

	mu      sync.Mutex
	clients map[*client]struct{}
	watches map[string]*watch
	nextSeq int

	// allowControl is whether a client that is not this machine's own window may
	// be given a shell's lease. It is read and written under mu, like everything
	// else a client's frame can change while other clients are being served.
	allowControl bool
}

// watch is the hub-side attachment state for one session.
type watch struct {
	controller *client
	viewers    []*client

	// epoch counts every change of who holds the lease. Input, resize and
	// close carry the epoch they were sent under; a frame whose epoch is not
	// the current one belonged to a holder that has since been replaced and is
	// dropped, so nothing lands after a handoff.
	epoch uint64

	// deadline is when the holder's lease lapses. Every input renews it, and
	// the janitor releases the lease once it passes, so a controller whose
	// network died cannot lock the machine out of its own shell.
	deadline  time.Time
	lastInput time.Time
}

// held reports whether the lease is held right now: a controller that exists
// and whose deadline has not passed.
func (w *watch) held(now time.Time) bool {
	return w.controller != nil && now.Before(w.deadline)
}

// take moves the lease to c, whoever held it before. The epoch is bumped so
// the previous holder's in-flight input dies with the handoff.
func (w *watch) take(c *client, now time.Time, ttl time.Duration) (prev *client) {
	prev = w.controller
	if prev != nil && prev != c {
		w.viewers = append(w.viewers, prev)
	}
	w.removeViewer(c)
	w.controller = c
	w.epoch++
	w.lastInput, w.deadline = now, now.Add(ttl)
	return prev
}

// drop releases the lease, whoever held it, and bumps the epoch so the
// departing holder's frames stop landing.
func (w *watch) drop(now time.Time) (prev *client) {
	prev = w.controller
	if prev != nil {
		w.controller = nil
		if !w.hasViewer(prev) {
			w.viewers = append(w.viewers, prev)
		}
	}
	w.epoch++
	w.deadline = time.Time{}
	return prev
}

// renew pushes the holder's deadline out on input it actually sent.
func (w *watch) renew(now time.Time, ttl time.Duration) {
	w.lastInput = now
	w.deadline = now.Add(ttl)
}

func New(opts Options) *Hub {
	idle := opts.IdleTimeout
	if idle <= 0 {
		idle = DefaultIdleController
	}
	origins := opts.OriginPatterns
	if len(origins) == 0 {
		origins = nil
	}
	return &Hub{
		mgr:          session.NewManager(),
		opts:         opts,
		host:         opts.Host,
		idle:         idle,
		allowControl: !opts.ControlOff,
		clients:      map[*client]struct{}{},
		watches:      map[string]*watch{},
	}
}

// Manager exposes the session set, mainly so a caller can shut it down.
func (h *Hub) Manager() *session.Manager { return h.mgr }

// ControlAllowed reports whether a client that is not this machine's own window
// may be given a shell's lease.
func (h *Hub) ControlAllowed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.allowControl
}

// SetControlAllowed turns other clients' control of this machine on or off.
//
// The machine's own window is never affected — it is the machine's shell, and a
// switch on a machine must not lock the machine out of itself. Turning it off
// takes the lease from whoever else holds one, drops that client to a viewer and
// bumps the epoch in the same frame, so a controller that was just revoked
// cannot land one more keystroke; a client that claims afterwards, or asks for a
// shell of its own, is refused with ControlOff and can still watch.
func (h *Hub) SetControlAllowed(on bool) {
	type revoked struct {
		id    string
		who   *client
		epoch uint64
	}
	h.mu.Lock()
	if h.allowControl == on {
		h.mu.Unlock()
		return
	}
	h.allowControl = on
	var lost []revoked
	if !on {
		now := time.Now()
		for id, w := range h.watches {
			if w.controller == nil || w.controller.owner {
				continue
			}
			prev := w.drop(now)
			if prev != nil {
				lost = append(lost, revoked{id: id, who: prev, epoch: w.epoch})
			}
		}
	}
	h.mu.Unlock()

	for _, r := range lost {
		r.who.send(roleMsg(r.id, protocol.RoleViewer, "", r.epoch))
	}
	for _, r := range lost {
		h.broadcastRole(r.id)
	}
	if len(lost) > 0 {
		h.broadcastSessions()
	}
}

// mayControl reports whether c may be given a shell's lease. The machine's own
// window always may, because it is the machine; every other client may while
// this machine allows being driven from elsewhere. Callers hold mu.
func (h *Hub) mayControl(c *client) bool {
	return c.owner || h.allowControl
}

// Run drives the janitor that releases idle controllers until ctx is done.
func (h *Hub) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.reapIdleControllers()
		}
	}
}

// Reap runs one pass of the janitor. Run drives it every minute; a test that
// needs a lease to lapse now calls this rather than sleeping out the timeout.
func (h *Hub) Reap() { h.reapIdleControllers() }

// reapIdleControllers releases every lease whose deadline has passed. It is the
// same janitor as before the lease existed, now keyed on the deadline the
// holder's own input renews: a silent controller loses the shell rather than
// locking everyone out of it.
func (h *Hub) reapIdleControllers() {
	type eviction struct {
		c     *client
		id    string
		epoch uint64
	}
	var out []eviction

	h.mu.Lock()
	now := time.Now()
	for id, w := range h.watches {
		// Nothing to release without a holder, and nothing to release while the
		// holder's lease is live: its own input is renewing it.
		if w.controller == nil || w.held(now) {
			continue
		}
		c := w.drop(now)
		out = append(out, eviction{c: c, id: id, epoch: w.epoch})
	}
	h.mu.Unlock()

	for _, e := range out {
		e.c.send(roleMsg(e.id, protocol.RoleViewer, "", e.epoch))
		h.broadcastRole(e.id)
	}
}

// broadcastRole tells every client watching a session what the lease looks
// like now and what it means for them, so a client that was not the one
// demoted still learns it may claim.
func (h *Hub) broadcastRole(id string) {
	h.mu.Lock()
	w := h.watches[id]
	if w == nil {
		h.mu.Unlock()
		return
	}
	// Each watcher is told its own role: a viewer that is sent the
	// controller's role would believe it holds a shell it cannot type into.
	msgs := make([]struct {
		c   *client
		msg protocol.Msg
	}, 0, len(w.viewers)+1)
	for _, c := range w.attached() {
		msgs = append(msgs, struct {
			c   *client
			msg protocol.Msg
		}{c, roleMsg(id, w.roleOf(c), w.controllerID(), w.epoch)})
	}
	h.mu.Unlock()

	for _, m := range msgs {
		m.c.send(m.msg)
	}
}

// ServeHTTP upgrades the request and serves one client for its lifetime.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.opts.Token != "" && !h.tokenOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.opts.OriginPatterns,
	})
	if err != nil {
		return
	}
	h.serve(wsTransport{conn})
}

func (h *Hub) tokenOK(r *http.Request) bool {
	got := r.URL.Query().Get("token")
	if got == "" {
		got = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.opts.Token)) == 1
}

// ServeTransport attaches one client over an arbitrary transport and returns
// when it goes away. The relay agent uses this to project a remote viewer onto
// this hub, so a session controlled from the internet is arbitrated by exactly
// the same code as one controlled from the same machine.
func (h *Hub) ServeTransport(tp Transport) {
	h.serve(tp)
}

// ServeLocal attaches the machine's own client — the desktop window's — over an
// in-process transport. It is an ordinary client in every way (the same frames,
// the same arbitration) with one difference recorded: it is the owner, so
// taking a shell back from a remote controller needs no confirmation.
func (h *Hub) ServeLocal(tp Transport) {
	h.serveClient(h.ownerClient(tp))
}

func (h *Hub) serve(tp Transport) {
	h.serveClient(h.newClient(tp))
}

func (h *Hub) serveClient(c *client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); c.writeLoop(ctx) }()
	go func() { defer wg.Done(); c.heartbeat(ctx) }()

	c.readLoop(ctx)

	cancel()
	c.close("connection closed")
	wg.Wait()
	h.removeClient(c)
}

// handle routes one decoded frame.
func (h *Hub) handle(c *client, m protocol.Msg) {
	switch m.T {
	case protocol.CTHello:
		if m.V != 0 && m.V != protocol.Version {
			c.fail(protocol.ErrProtocol, fmt.Sprintf(
				"protocol version %d is not supported (this daemon speaks %d)", m.V, protocol.Version), "")
			return
		}
		host := h.host
		c.send(protocol.Msg{
			T:        protocol.STHello,
			V:        protocol.Version,
			Host:     &host,
			Sessions: h.listSummaries(),
		})
	case protocol.CTList:
		c.send(protocol.Msg{
			T:        protocol.STSessions,
			ReqID:    m.ReqID,
			Sessions: h.listSummaries(),
		})
	case protocol.CTCreate:
		h.handleCreate(c, m)
	case protocol.CTAttach:
		h.handleAttach(c, m.SessionID, m.Cursor)
	case protocol.CTDetach:
		h.handleDetach(c, m.SessionID)
	case protocol.CTInput:
		h.handleInput(c, m.SessionID, m.Data, m.Epoch)
	case protocol.CTResize:
		h.handleResize(c, m.SessionID, m.Cols, m.Rows, m.Epoch)
	case protocol.CTClaim:
		h.handleClaim(c, m.SessionID, m.Steal)
	case protocol.CTRelease:
		h.handleRelease(c, m.SessionID)
	case protocol.CTClose:
		h.handleClose(c, m.SessionID, m.Epoch)
	case "":
		c.fail(protocol.ErrProtocol, "frame has no type field", "")
	default:
		c.fail(protocol.ErrProtocol, "unknown frame type: "+m.T, m.SessionID)
	}
}

func (h *Hub) handleCreate(c *client, m protocol.Msg) {
	// A shell on this machine is this machine's to hand out: with control off, a
	// client that may not type into a shell may not start one either.
	h.mu.Lock()
	allowed := h.mayControl(c)
	h.mu.Unlock()
	if !allowed {
		c.fail(protocol.ErrControlOff,
			"this machine does not allow other clients to start shells on it", m.SessionID)
		return
	}
	// The session is made quietly and announced in order: the client that asked
	// for it is told which shell it got before the session list puts that shell
	// in front of everyone else, so its pending tab becomes the tab watching it
	// rather than a second tab for the same shell.
	id := m.SessionID
	if id == "" {
		id = h.newSessionID()
	}
	s, err := h.start(id, m.Cols, m.Rows, m.Cwd, m.Shell)
	if err != nil {
		c.fail(protocol.ErrBadRequest, err.Error(), id)
		return
	}
	c.send(protocol.Msg{T: protocol.STCreated, SessionID: s.ID})
	h.attach(c, s, 0)
}

func (h *Hub) handleAttach(c *client, id string, cursor int64) {
	s, ok := h.mgr.Get(id)
	if !ok {
		c.fail(protocol.ErrNoSession, "no live session named "+id, id)
		return
	}
	h.attach(c, s, cursor)
}

// attach registers c as a watcher of s and catches it up to cursor.
//
// A client that is already watching keeps its role and only moves its cursor.
// Otherwise the first watcher of an unclaimed session becomes the controller
// and everyone after it is a viewer.
func (h *Hub) attach(c *client, s *session.Session, cursor int64) {
	if cursor < 0 {
		cursor = 0
	}

	h.mu.Lock()
	w := h.watches[s.ID]
	if w == nil {
		w = &watch{lastInput: time.Now()}
		h.watches[s.ID] = w
	}

	promoted := false
	now := time.Now()
	switch {
	case w.controller == c:
		w.renew(now, h.idle)
	case w.hasViewer(c):
		// Already watching; only the cursor moves.
	case !w.held(now) && h.mayControl(c):
		// Unclaimed, or the holder's lease has lapsed: the first watcher takes
		// it, as the machine's own window does when it opens a shell. A machine
		// that allows no other clients to control it leaves them watching.
		w.take(c, now, h.idle)
		promoted = true
	default:
		w.viewers = append(w.viewers, c)
	}

	data, from, truncated := s.Ring().ReadFrom(cursor)
	next := from + int64(len(data))
	c.setCursor(s.ID, next)

	sum := h.summaryLocked(s)
	c.send(protocol.Msg{
		T:         protocol.STAttached,
		SessionID: s.ID,
		Session:   &sum,
		Cursor:    next,
		Truncated: truncated,
	})
	if len(data) > 0 {
		c.send(protocol.Msg{
			T:         protocol.STOutput,
			SessionID: s.ID,
			Cursor:    next,
			Data:      data,
			Resync:    truncated,
		})
	}
	role := roleMsg(s.ID, w.roleOf(c), w.controllerID(), w.epoch)
	var peers []*client
	if promoted {
		peers = w.others(c)
	}
	h.mu.Unlock()

	c.send(role)
	for _, p := range peers {
		p.send(roleMsg(s.ID, protocol.RoleViewer, c.id, w.epoch))
	}
	h.broadcastSessions()
}

func (h *Hub) handleDetach(c *client, id string) {
	h.mu.Lock()
	w := h.watches[id]
	freed := w != nil && w.controller == c
	if w != nil {
		if freed {
			// A client that stops watching cannot hold the lease: it is
			// released, and everyone still watching is told, as they are when
			// the holder's connection drops.
			w.drop(time.Now())
		}
		w.removeViewer(c)
	}
	h.mu.Unlock()
	// The session itself is untouched: that is the whole point of detach.
	if freed {
		h.broadcastRole(id)
	}
	h.broadcastSessions()
}

func (h *Hub) handleInput(c *client, id string, data []byte, epoch uint64) {
	s, ok := h.mgr.Get(id)
	if !ok {
		c.fail(protocol.ErrNoSession, "no live session named "+id, id)
		return
	}
	switch h.controlFor(c, id, epoch) {
	case controlHeld:
		if err := s.Write(data); err != nil {
			c.fail(protocol.ErrNoSession, err.Error(), id)
		}
	case controlLost:
		// Answered loudly: the client is a viewer and can be offered the lease.
		c.fail(protocol.ErrNotController,
			"another client controls "+id+"; claim it first", id)
	case controlStale:
		// Dropped in silence. This client held the lease a moment ago and has
		// already been told it lost it; its frame belonged to the handoff.
	}
}

func (h *Hub) handleResize(c *client, id string, cols, rows int, epoch uint64) {
	s, ok := h.mgr.Get(id)
	if !ok {
		c.fail(protocol.ErrNoSession, "no live session named "+id, id)
		return
	}
	if h.controlFor(c, id, epoch) != controlHeld {
		return
	}
	if err := s.Resize(cols, rows); err != nil {
		c.fail(protocol.ErrBadSize, err.Error(), id)
	}
}

// control is what a frame bearing an epoch is entitled to.
type control int

const (
	controlHeld  control = iota // the sender holds the lease, at this epoch
	controlLost                 // the sender does not hold the lease at all
	controlStale                // the sender held it, at an epoch now past
)

// controlFor decides what a frame from c may do, renewing the lease when it
// may act. Input from a viewer is answered loudly rather than dropped, so a
// client can offer "take control"; input from a holder that has just been
// replaced is dropped, because the role frame already told it so.
//
// A frame carrying no epoch is from a client that does not speak the lease at
// all: it is checked against the controller alone, exactly as every client was
// before the lease existed. Epochs start at 1, so a zero is never a current
// one and the two cannot be confused.
func (h *Hub) controlFor(c *client, id string, epoch uint64) control {
	h.mu.Lock()
	defer h.mu.Unlock()
	w := h.watches[id]
	if w == nil || w.controller != c {
		return controlLost
	}
	if epoch != 0 && epoch != w.epoch {
		return controlStale
	}
	w.renew(time.Now(), h.idle)
	return controlHeld
}

// requireControl reports whether c may act on the session at the given epoch,
// answering the caller with a not_controller error when it may not. Input from
// a viewer is rejected loudly rather than dropped, so a client can offer "take
// control".
func (h *Hub) requireControl(c *client, id string, epoch uint64) bool {
	if h.controlFor(c, id, epoch) == controlHeld {
		return true
	}
	c.fail(protocol.ErrNotController,
		"another client controls "+id+"; claim it first", id)
	return false
}

// handleClaim takes the lease. A claim for a lease nobody holds, or one whose
// deadline has passed, is honoured at once. A claim against a live holder is
// refused with ErrLeaseHeld unless it carries Steal — taking a shell from
// under someone is a thing a person asks for, not something a stray click
// does. The machine the PTY runs on is the exception its own window needs: its
// claim is always honoured, because it is the machine's shell.
func (h *Hub) handleClaim(c *client, id string, steal bool) {
	if _, ok := h.mgr.Get(id); !ok {
		c.fail(protocol.ErrNoSession, "no live session named "+id, id)
		return
	}

	h.mu.Lock()
	w := h.watches[id]
	if w == nil {
		w = &watch{}
		h.watches[id] = w
	}
	now := time.Now()
	var prev *client
	switch {
	case w.controller == c:
		// Already mine: a claim is a renewal, and the epoch does not move.
		w.renew(now, h.idle)
	case !h.mayControl(c):
		// This machine does not allow other clients to control it. The claim is
		// answered rather than dropped, so the client can say so on screen.
		h.mu.Unlock()
		c.fail(protocol.ErrControlOff,
			"this machine does not allow other clients to control its shells", id)
		return
	case !w.held(now) || steal || c.owner:
		prev = w.take(c, now, h.idle)
	default:
		holder := w.controllerID()
		until := w.deadline.Format("15:04:05")
		h.mu.Unlock()
		c.fail(protocol.ErrLeaseHeld,
			holder+" holds this shell until "+until+"; steal it to take it anyway", id)
		return
	}
	epoch := w.epoch
	h.mu.Unlock()

	if prev != nil && prev != c {
		prev.send(roleMsg(id, protocol.RoleViewer, c.id, epoch))
	}
	c.send(roleMsg(id, protocol.RoleController, c.id, epoch))
	h.broadcastRole(id)
	h.broadcastSessions()
}

func (h *Hub) handleRelease(c *client, id string) {
	h.mu.Lock()
	w := h.watches[id]
	var prev *client
	epoch := uint64(0)
	if w != nil && w.controller == c {
		prev = w.drop(time.Now())
		epoch = w.epoch
	}
	h.mu.Unlock()

	if prev != nil {
		prev.send(roleMsg(id, protocol.RoleViewer, "", epoch))
		h.broadcastRole(id)
		h.broadcastSessions()
	}
}

func (h *Hub) handleClose(c *client, id string, epoch uint64) {
	s, ok := h.mgr.Get(id)
	if !ok {
		c.fail(protocol.ErrNoSession, "no live session named "+id, id)
		return
	}
	// Closing kills the process for everyone, so it belongs to whoever holds
	// control. A client that only wants to stop looking sends detach.
	if !h.requireControl(c, id, epoch) {
		return
	}
	_ = s.Close()
	c.send(protocol.Msg{T: protocol.STClosed, SessionID: id, Reason: "requested"})
}

// watchSession flushes output to watchers and finalises the session once its
// process exits. One goroutine per session.
func (h *Hub) watchSession(s *session.Session) {
	for {
		select {
		case <-s.Changes():
			h.flush(s)
		case <-s.Exited():
			h.flush(s)
			h.onSessionExit(s)
			return
		}
	}
}

// flush copies whatever each watcher has not seen yet, using their cursor as
// the starting point. Nothing is queued per byte, so a slow client cannot make
// the daemon buffer, and a reconnecting client self-heals in one step.
func (h *Hub) flush(s *session.Session) {
	h.mu.Lock()
	w := h.watches[s.ID]
	var targets []*client
	if w != nil {
		targets = w.attached()
	}
	for _, c := range targets {
		cursor := c.cursor(s.ID)
		data, from, truncated := s.Ring().ReadFrom(cursor)
		if len(data) == 0 && !truncated {
			continue
		}
		next := from + int64(len(data))
		c.setCursor(s.ID, next)
		c.send(protocol.Msg{
			T:         protocol.STOutput,
			SessionID: s.ID,
			Cursor:    next,
			Data:      data,
			Resync:    truncated,
		})
	}
	h.mu.Unlock()
}

func (h *Hub) onSessionExit(s *session.Session) {
	code := s.ExitCode()
	h.mgr.Forget(s.ID)

	h.mu.Lock()
	w := h.watches[s.ID]
	delete(h.watches, s.ID)
	var targets []*client
	if w != nil {
		targets = w.attached()
	}
	h.mu.Unlock()

	msg := protocol.Msg{T: protocol.STExit, SessionID: s.ID, ExitCode: code}
	for _, c := range targets {
		c.send(msg)
	}
	h.broadcastSessions()
}

// removeClient detaches a departing client from every session it was watching.
// None of those sessions are closed. A departing controller's lease is
// released and the epoch bumped, so the clients still watching learn in the
// same frame that the shell is free to claim.
func (h *Hub) removeClient(c *client) {
	h.mu.Lock()
	delete(h.clients, c)
	type freed struct {
		id    string
		epoch uint64
	}
	var freedList []freed
	for id, w := range h.watches {
		hadController := w.controller == c
		wasViewer := w.hasViewer(c)
		if hadController {
			w.drop(time.Now())
			freedList = append(freedList, freed{id: id, epoch: w.epoch})
		}
		if wasViewer {
			w.removeViewer(c)
		}
	}
	h.mu.Unlock()

	c.close("connection closed")
	for _, f := range freedList {
		h.broadcastRole(f.id)
	}
	if len(freedList) > 0 {
		h.broadcastSessions()
	}
}

// broadcastSessions pushes the full list to everyone. It is sent on every
// change rather than diffed: the list is small, and a client that receives it
// wholesale cannot drift out of order with the host.
func (h *Hub) broadcastSessions() {
	sessions := h.listSummaries()
	h.mu.Lock()
	cs := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		cs = append(cs, c)
	}
	h.mu.Unlock()

	msg := protocol.Msg{T: protocol.STSessions, Sessions: sessions}
	for _, c := range cs {
		c.send(msg)
	}
}

func (h *Hub) listSummaries() []protocol.Summary {
	h.mu.Lock()
	defer h.mu.Unlock()

	sessions := h.mgr.List()
	out := make([]protocol.Summary, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, h.summaryLocked(s))
	}
	return out
}

func (h *Hub) summaryLocked(s *session.Session) protocol.Summary {
	sum := s.Summary()
	if w := h.watches[s.ID]; w != nil {
		sum.Controller = w.controllerID()
		for _, v := range w.viewers {
			sum.Viewers = append(sum.Viewers, v.id)
		}
	}
	return sum
}

func (h *Hub) newSessionID() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextSeq++
	return fmt.Sprintf("s%d", h.nextSeq)
}

// ---- watch helpers (callers hold h.mu) ----

func (w *watch) hasViewer(c *client) bool {
	for _, v := range w.viewers {
		if v == c {
			return true
		}
	}
	return false
}

func (w *watch) removeViewer(c *client) {
	out := w.viewers[:0]
	for _, v := range w.viewers {
		if v != c {
			out = append(out, v)
		}
	}
	w.viewers = out
}

// attached is the controller followed by the viewers.
func (w *watch) attached() []*client {
	out := make([]*client, 0, len(w.viewers)+1)
	if w.controller != nil {
		out = append(out, w.controller)
	}
	return append(out, w.viewers...)
}

func (w *watch) others(c *client) []*client {
	var out []*client
	for _, x := range w.attached() {
		if x != c {
			out = append(out, x)
		}
	}
	return out
}

func (w *watch) roleOf(c *client) string {
	if w.controller == c {
		return protocol.RoleController
	}
	return protocol.RoleViewer
}

func (w *watch) controllerID() string {
	if w.controller == nil {
		return ""
	}
	return w.controller.id
}

func roleMsg(id, role, controller string, epoch uint64) protocol.Msg {
	return protocol.Msg{
		T:          protocol.STRole,
		SessionID:  id,
		Role:       role,
		Controller: controller,
		Epoch:      epoch,
	}
}
