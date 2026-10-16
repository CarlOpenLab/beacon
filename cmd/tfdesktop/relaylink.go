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
}

// openRelayLink starts the stream to a machine and returns the window's end of
// it. The stream opens when the machine is online; until then the link exists
// and carries nothing, exactly as a browser's does.
func openRelayLink(c *relayclient.Client, machine string) *relayLink {
	l := &relayLink{link: newLink(), machine: machine}
	l.stream = c.Open(machine, relayHandlers{l: l})
	l.send = func(m protocol.Msg) bool { return l.stream.Send(m) }
	return l
}

// close ends the stream. The machine's shells are left alone.
func (l *relayLink) close() {
	l.stream.Close()
	l.link.close("link closed")
}

// relayHandlers routes one machine's stream to its link.
type relayHandlers struct{ l *relayLink }

func (h relayHandlers) Open() {
	// The stream is routed: say hello, so the hub sends its session list, and
	// the panel can show what this machine is running.
	h.l.sendMsg(protocol.Msg{T: protocol.CTHello, V: protocol.Version})
	h.l.sendMsg(protocol.Msg{T: protocol.CTList})
}

func (h relayHandlers) Frame(m protocol.Msg) { h.l.dispatch(m) }

func (h relayHandlers) Close(reason string) { h.l.link.close(reason) }
