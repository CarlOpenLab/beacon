package hub

import (
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/session"
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
// starting a second shell under the same name.
func (h *Hub) Create(id string, cols, rows int, cwd, shell string) (*session.Session, error) {
	if id == "" {
		id = h.newSessionID()
	}
	s, reused, err := h.mgr.Create(id, cols, rows, cwd, shell)
	if err != nil {
		return nil, err
	}
	if !reused {
		go h.watchSession(s)
	}
	h.broadcastSessions()
	return s, nil
}

// Sessions lists the live sessions in the shape clients receive them, in the
// same order, so a native list and a web list agree row for row.
func (h *Hub) Sessions() []protocol.Summary { return h.listSummaries() }

// Session returns a live session by id.
func (h *Hub) Session(id string) (*session.Session, bool) { return h.mgr.Get(id) }

// Kill ends a session's process. The session is forgotten once it is reaped,
// and its tab disappears from every client, the native window included.
func (h *Hub) Kill(id string) error { return h.mgr.Close(id) }
