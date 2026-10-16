package hub

import (
	"context"
	"sync"
)

// Pipe joins two ends of a Transport back to back inside one process: the hub
// serves one end, and the other is driven by a client that lives in the same
// program — the desktop window. It exists so the machine's own window can be
// an ordinary hub client, speaking the same frames and answering to the same
// arbitration as a browser on the phone, over no socket at all.
//
// The ends are symmetric and unbuffered in spirit: each side has a reader, so a
// frame handed one way is taken by the other without either side waiting on
// work the other has not started.
type Pipe struct {
	up   chan []byte // window -> hub
	down chan []byte // hub -> window
	done chan struct{}
	once sync.Once
}

// NewPipe makes a joined pair. Serve the hub end with ServeLocal and drive the
// other with Send and Recv.
func NewPipe() *Pipe {
	return &Pipe{
		up:   make(chan []byte, 64),
		down: make(chan []byte, 64),
		done: make(chan struct{}),
	}
}

// Local is the hub's end of the pipe, as the Transport the hub serves.
func (p *Pipe) Local() Transport { return pipeTransport{p} }

// Send hands the hub a frame from the window's end.
func (p *Pipe) Send(b []byte) error {
	select {
	case p.up <- b:
		return nil
	case <-p.done:
		return context.Canceled
	}
}

// Recv takes the next frame the hub sent the window, or the pipe's end.
func (p *Pipe) Recv(ctx context.Context) ([]byte, error) {
	select {
	case b := <-p.down:
		return b, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.done:
		return nil, context.Canceled
	}
}

// Close ends the pipe in both directions. The hub's client goes away with it,
// which releases every lease it held, as a dropped connection does.
func (p *Pipe) Close() error {
	p.once.Do(func() { close(p.done) })
	return nil
}

// pipeTransport is the hub's end: it reads what the window sends and writes
// what the window receives.
type pipeTransport struct{ p *Pipe }

func (t pipeTransport) Read(ctx context.Context) ([]byte, error) {
	select {
	case b := <-t.p.up:
		return b, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.p.done:
		return nil, context.Canceled
	}
}

func (t pipeTransport) Write(ctx context.Context, b []byte) error {
	select {
	case t.p.down <- b:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.p.done:
		return context.Canceled
	}
}

func (t pipeTransport) Ping(ctx context.Context) error { return nil }

func (t pipeTransport) Close(reason string) error { return t.p.Close() }
