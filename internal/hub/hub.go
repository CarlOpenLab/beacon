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
}

// watch is the hub-side attachment state for one session.
type watch struct {
	controller *client
	viewers    []*client
	lastInput  time.Time
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
		mgr:     session.NewManager(),
		opts:    opts,
		host:    opts.Host,
		idle:    idle,
		clients: map[*client]struct{}{},
		watches: map[string]*watch{},
	}
}

// Manager exposes the session set, mainly so a caller can shut it down.
func (h *Hub) Manager() *session.Manager { return h.mgr }

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

func (h *Hub) reapIdleControllers() {
	type eviction struct {
		c  *client
		id string
	}
	var out []eviction

	h.mu.Lock()
	now := time.Now()
	for id, w := range h.watches {
		if w.controller == nil || now.Sub(w.lastInput) <= h.idle {
			continue
		}
		c := w.controller
		w.controller = nil
		w.viewers = append(w.viewers, c)
		out = append(out, eviction{c: c, id: id})
	}
	h.mu.Unlock()

	if len(out) == 0 {
		return
	}
	for _, e := range out {
		e.c.send(roleMsg(e.id, protocol.RoleViewer, ""))
	}
	h.broadcastSessions()
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

func (h *Hub) serve(tp Transport) {
	c := h.newClient(tp)

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
		h.handleInput(c, m.SessionID, m.Data)
	case protocol.CTResize:
		h.handleResize(c, m.SessionID, m.Cols, m.Rows)
	case protocol.CTClaim:
		h.handleClaim(c, m.SessionID)
	case protocol.CTRelease:
		h.handleRelease(c, m.SessionID)
	case protocol.CTClose:
		h.handleClose(c, m.SessionID)
	case "":
		c.fail(protocol.ErrProtocol, "frame has no type field", "")
	default:
		c.fail(protocol.ErrProtocol, "unknown frame type: "+m.T, m.SessionID)
	}
}

func (h *Hub) handleCreate(c *client, m protocol.Msg) {
	id := m.SessionID
	if id == "" {
		id = h.newSessionID()
	}
	s, reused, err := h.mgr.Create(id, m.Cols, m.Rows, m.Cwd, m.Shell)
	if err != nil {
		c.fail(protocol.ErrBadRequest, err.Error(), id)
		return
	}
	if !reused {
		go h.watchSession(s)
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
	switch {
	case w.controller == c:
		w.lastInput = time.Now()
	case w.hasViewer(c):
		// Already watching; only the cursor moves.
	case w.controller == nil:
		w.controller = c
		w.lastInput = time.Now()
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
	role := roleMsg(s.ID, w.roleOf(c), w.controllerID())
	var peers []*client
	if promoted {
		peers = w.others(c)
	}
	h.mu.Unlock()

	c.send(role)
	for _, p := range peers {
		p.send(roleMsg(s.ID, protocol.RoleViewer, c.id))
	}
	h.broadcastSessions()
}

func (h *Hub) handleDetach(c *client, id string) {
	h.mu.Lock()
	if w := h.watches[id]; w != nil {
		if w.controller == c {
			w.controller = nil
		}
		w.removeViewer(c)
	}
	h.mu.Unlock()
	// The session itself is untouched: that is the whole point of detach.
	h.broadcastSessions()
}

func (h *Hub) handleInput(c *client, id string, data []byte) {
	s, ok := h.mgr.Get(id)
	if !ok {
		c.fail(protocol.ErrNoSession, "no live session named "+id, id)
		return
	}
	if !h.requireControl(c, id) {
		return
	}
	if err := s.Write(data); err != nil {
		c.fail(protocol.ErrNoSession, err.Error(), id)
	}
}

func (h *Hub) handleResize(c *client, id string, cols, rows int) {
	s, ok := h.mgr.Get(id)
	if !ok {
		c.fail(protocol.ErrNoSession, "no live session named "+id, id)
		return
	}
	if !h.requireControl(c, id) {
		return
	}
	if err := s.Resize(cols, rows); err != nil {
		c.fail(protocol.ErrBadSize, err.Error(), id)
	}
}

// requireControl reports whether c may act on the session, answering the
// caller with a not_controller error when it may not. Input from a viewer is
// rejected loudly rather than dropped, so a client can offer "take control".
func (h *Hub) requireControl(c *client, id string) bool {
	h.mu.Lock()
	w := h.watches[id]
	ok := w != nil && w.controller == c
	if ok {
		w.lastInput = time.Now()
	}
	h.mu.Unlock()
	if !ok {
		c.fail(protocol.ErrNotController,
			"another client controls "+id+"; claim it first", id)
	}
	return ok
}

func (h *Hub) handleClaim(c *client, id string) {
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
	prev := w.controller
	if prev != c {
		if prev != nil {
			w.viewers = append(w.viewers, prev)
		}
		w.removeViewer(c)
		w.controller = c
	}
	w.lastInput = time.Now()
	h.mu.Unlock()

	if prev != nil && prev != c {
		prev.send(roleMsg(id, protocol.RoleViewer, c.id))
	}
	c.send(roleMsg(id, protocol.RoleController, c.id))
	h.broadcastSessions()
}

func (h *Hub) handleRelease(c *client, id string) {
	h.mu.Lock()
	w := h.watches[id]
	wasController := w != nil && w.controller == c
	if wasController {
		w.controller = nil
		if !w.hasViewer(c) {
			w.viewers = append(w.viewers, c)
		}
	}
	h.mu.Unlock()

	if wasController {
		c.send(roleMsg(id, protocol.RoleViewer, ""))
		h.broadcastSessions()
	}
}

func (h *Hub) handleClose(c *client, id string) {
	s, ok := h.mgr.Get(id)
	if !ok {
		c.fail(protocol.ErrNoSession, "no live session named "+id, id)
		return
	}
	// Closing kills the process for everyone, so it belongs to whoever holds
	// control. A client that only wants to stop looking sends detach.
	if !h.requireControl(c, id) {
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
// None of those sessions are closed.
func (h *Hub) removeClient(c *client) {
	h.mu.Lock()
	delete(h.clients, c)
	touched := 0
	for _, w := range h.watches {
		hadController := w.controller == c
		wasViewer := w.hasViewer(c)
		if hadController {
			w.controller = nil
		}
		if wasViewer {
			w.removeViewer(c)
		}
		if hadController || wasViewer {
			touched++
		}
	}
	h.mu.Unlock()

	c.close("connection closed")
	if touched > 0 {
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

func roleMsg(id, role, controller string) protocol.Msg {
	return protocol.Msg{
		T:          protocol.STRole,
		SessionID:  id,
		Role:       role,
		Controller: controller,
	}
}
