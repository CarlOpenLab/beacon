package hub

import (
	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/internal/session"
)

// This file is the host-facing edge of the hub: the small surface a program
// that owns sessions in-process uses, instead of speaking the client protocol
// to itself. The desktop window is such a program.
//
// Sessions made here are ordinary hub sessions. They are watched and
// broadcast like any other, so a tab in the native window and a tab in a
// browser are the same session seen twice, one of the knobs being a real PTY
// on this machine.

// Create starts a session and begins watching it, so it reaches every
// attached client and appears in the session list. An empty id is replaced by
// a fresh one. An id that names a live session returns that session instead of
// starting a second shell under the same name. from names a live session whose
// working directory the new shell opens in; see protocol.Msg.From.
func (h *Hub) Create(id string, cols, rows int, cwd, shell, from string) (*session.Session, error) {
	s, err := h.start(id, cols, rows, cwd, shell, from)
	if err != nil {
		return nil, err
	}
	h.broadcastSessions()
	return s, nil
}

// start makes the session without telling anyone about it. A caller that speaks
// the client protocol announces the session itself, in the order its client
// expects: the client that asked for it hears first, so its tab is the tab
// watching the shell before the session list puts the shell in front of
// everyone else.
func (h *Hub) start(id string, cols, rows int, cwd, shell, from string) (*session.Session, error) {
	if id == "" {
		id = h.newSessionID()
	}
	cwd = h.inheritDir(from, cwd)
	s, reused, err := h.mgr.Create(id, cols, rows, cwd, shell)
	if err != nil {
		return nil, err
	}
	if !reused {
		go h.watchSession(s)
	}
	return s, nil
}

// Sessions lists the live sessions in the shape clients receive them, in the
// same order, so a native list and a web list agree row for row.
func (h *Hub) Sessions() []protocol.Summary { return h.listSummaries() }

// Session returns a live session by id.
func (h *Hub) Session(id string) (*session.Session, bool) { return h.mgr.Get(id) }

// inheritDir is the directory a new shell opens in. from names a live session
// of this machine's: the new shell opens where that one stands now, so a split
// or a second tab continues where the shell beside it is rather than at the
// home directory. A session that is gone, and a machine that cannot say where
// its processes stand, leave cwd as the client sent it — and a client that sent
// nothing gets the home directory, which is session.Start's own answer.
func (h *Hub) inheritDir(from, cwd string) string {
	if from == "" {
		return cwd
	}
	src, ok := h.mgr.Get(from)
	if !ok || !src.Alive() {
		return cwd
	}
	if live := src.LiveCwd(); live != "" {
		return live
	}
	return cwd
}

// Kill ends a session's process. The session is forgotten once it is reaped,
// and its tab disappears from every client, the native window included.
func (h *Hub) Kill(id string) error { return h.mgr.Close(id) }
