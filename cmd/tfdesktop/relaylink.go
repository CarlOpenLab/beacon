package main

// One remote machine, reached through the relay.
//
// The relay carries the session protocol inside its own frames, so a remote
// machine's hub speaks to this window exactly as the local one does: the same
// frames, the same arbitration, the same lease. The window is a client of that
// machine's hub — not of the relay, which routes bytes and decides nothing —
// which is why taking over a shell on another machine is the same act as taking
// one back on this one.

import (
	"sync"

	"github.com/CarlOpenLab/term-fleet/internal/diag"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/relayclient"
)

// relayLink is the window's client end of one machine's hub, over a relay
// stream. It shares the session logic with the local link: a link is a link,
// and only the wire under it differs.
type relayLink struct {
	*link
	stream  *relayclient.Stream
	machine string

	// mu guards queued, which holds the frames written before the stream was
	// routed. A stream's Send fails until the relay has accepted it — the relay
	// has to tell the machine who is asking before it routes anything — while the
	// window writes the moment it decides something: open a terminal there, look
	// at what it runs. Those frames wait here for the accept instead of being
	// dropped on the floor.
	mu     sync.Mutex
	queued []protocol.Msg
}

// maxQueued bounds what one unrouted link holds. The window writes a handful of
// frames per machine at most, so a queue this long means the relay never routed
// the stream and the frames are stale rather than early.
const maxQueued = 64

// openRelayLink starts the stream to a machine and returns the window's end of
// it. The stream opens when the machine is online; until then the link exists
// and carries nothing, exactly as a browser's does.
func openRelayLink(c *relayclient.Client, machine string) *relayLink {
	l := &relayLink{link: newLink(), machine: machine}
	l.name = machine
	l.stream = c.Open(machine, relayHandlers{l: l})
	l.setSend(l.write)
	diag.Infof("relay", "stream opening machine=%s", machine)
	return l
}

// write sends a frame, or holds it until the stream is routed.
func (l *relayLink) write(m protocol.Msg) bool {
	if l.stream.Send(m) {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.queued) >= maxQueued {
		// The queue is full, so this frame — possibly the attach that would
		// paint a pane — is not going anywhere. That used to be a silent no-op;
		// it is the shape of a blank pane that never recovers while the machine
		// is reachable, so it is a warning with the frame named.
		diag.Warnf("relay", "frame refused: stream not routed machine=%s type=%s session=%s queued=%d",
			l.machine, m.T, m.SessionID, len(l.queued))
		return false
	}
	l.queued = append(l.queued, m)
	diag.Debugf("relay", "frame queued machine=%s type=%s session=%s queued=%d",
		l.machine, m.T, m.SessionID, len(l.queued))
	return true
}

// flush sends what was written before the stream was routed, in the order it was
// written. A frame that cannot be sent belongs to a stream that has just gone
// away; it is dropped, because the machine is unreachable and the panel says so
// rather than the link pretending the frame is still pending.
func (l *relayLink) flush() {
	l.mu.Lock()
	queued := l.queued
	l.queued = nil
	l.mu.Unlock()
	if len(queued) > 0 {
		diag.Infof("relay", "flushing machine=%s frames=%d", l.machine, len(queued))
	}
	for i, m := range queued {
		if !l.stream.Send(m) {
			diag.Warnf("relay", "flush stopped machine=%s at=%d of=%d", l.machine, i, len(queued))
			return
		}
	}
}

// close ends the stream. The machine's shells are left alone.
func (l *relayLink) close() {
	diag.Infof("relay", "stream closing machine=%s", l.machine)
	l.dropQueued()
	l.stream.Close()
	l.link.close("link closed")
}

// dropQueued forgets the frames that were waiting for a stream that is gone.
func (l *relayLink) dropQueued() {
	l.mu.Lock()
	l.queued = nil
	l.mu.Unlock()
}

// relayHandlers routes one machine's stream to its link.
type relayHandlers struct{ l *relayLink }

func (h relayHandlers) Open() {
	diag.Infof("relay", "stream open machine=%s", h.l.machine)
	// The stream is routed: say hello, so the hub sends its session list, and
	// the panel can show what this machine is running. What the window wrote
	// while it was waiting goes out behind the hello: a machine's hub is a plain
	// client of its own protocol, and it expects to be greeted first.
	h.l.sendMsg(protocol.Msg{T: protocol.CTHello, V: protocol.Version})
	h.l.sendMsg(protocol.Msg{T: protocol.CTList})
	h.l.flush()
	// A link that replaces a dead one picks the orphaned tabs back up here.
	// This runs on the relay's reader goroutine, so the hook is the window's to
	// make thread-safe.
	if h.l.onOpen != nil {
		h.l.onOpen()
	}
}

func (h relayHandlers) Frame(m protocol.Msg) { h.l.dispatch(m) }

func (h relayHandlers) Close(reason string) {
	diag.Infof("relay", "stream closed machine=%s reason=%q", h.l.machine, reason)
	// The stream is gone, so nothing written for it is still worth holding: the
	// window removes the link and the panel stops offering the machine.
	h.l.dropQueued()
	h.l.link.close(reason)
}
