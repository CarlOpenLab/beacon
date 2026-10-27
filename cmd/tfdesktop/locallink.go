package main

// The window's own connection to this machine's hub.
//
// It is an ordinary hub client — the same frames a browser sends, the same
// arbitration — over an in-process pipe rather than a socket. The hub knows it
// is the machine's own window, which is the one thing that distinguishes it:
// its claim is always honoured, because a shell on this machine is the
// machine's to take back.

import (
	"context"

	"github.com/CarlOpenLab/beacon/internal/diag"
	"github.com/CarlOpenLab/beacon/internal/hub"
	"github.com/CarlOpenLab/beacon/internal/protocol"
)

// localLink is the window's client end of this machine's hub.
type localLink struct {
	*link
	pipe *hub.Pipe
}

// openLocalLink serves the machine's own client on the hub and returns the
// window's end of it, ready to speak the protocol.
func openLocalLink(h *hub.Hub) *localLink {
	pipe := hub.NewPipe()
	go h.ServeLocal(pipe.Local())

	l := &localLink{link: newLink(), pipe: pipe}
	l.name = machineLocal
	// The link's own wire is up as soon as the pipe is handed to the hub: there
	// is no handshake to wait for, unlike a relay stream.
	diag.Infof("link", "local link open")
	l.setSend(func(m protocol.Msg) bool {
		b, err := encode(m)
		if err != nil {
			return false
		}
		return pipe.Send(b) == nil
	})
	go l.readLoop(pipe.Recv)
	l.sendMsg(protocol.Msg{T: protocol.CTHello, V: protocol.Version,
		Caps: []string{protocol.CapFile, protocol.CapDeflate}})
	return l
}

// close ends the window's connection to its own hub. Every session it was
// watching is told, and the hub releases the leases it held.
func (l *localLink) close(reason string) {
	l.link.close(reason)
	_ = l.pipe.Close()
}

// readLoop decodes frames from the pipe until it ends.
func (l *localLink) readLoop(recv func(context.Context) ([]byte, error)) {
	for {
		b, err := recv(context.Background())
		if err != nil {
			l.link.close("local link ended")
			return
		}
		m, err := decode(b)
		if err != nil {
			continue
		}
		l.dispatch(m)
	}
}
