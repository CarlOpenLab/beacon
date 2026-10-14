package hub

import (
	"context"

	"github.com/coder/websocket"
)

// Transport is the link one client speaks over. Each Read returns exactly one
// frame; each Write sends exactly one.
//
// ServeHTTP uses a WebSocket. The relay agent supplies an in-process pipe
// instead, so a viewer sitting behind a public relay attaches to this hub
// through the very same path a local browser takes — the hub never learns
// whether the client is next to it or across the internet. That keeps one
// copy of the control arbitration instead of one per transport.
type Transport interface {
	Read(ctx context.Context) ([]byte, error)
	Write(ctx context.Context, b []byte) error
	Ping(ctx context.Context) error
	Close(reason string) error
}

// wsTransport adapts a WebSocket connection to Transport.
type wsTransport struct {
	conn *websocket.Conn
}

func (t wsTransport) Read(ctx context.Context) ([]byte, error) {
	_, data, err := t.conn.Read(ctx)
	return data, err
}

func (t wsTransport) Write(ctx context.Context, b []byte) error {
	return t.conn.Write(ctx, websocket.MessageText, b)
}

func (t wsTransport) Ping(ctx context.Context) error { return t.conn.Ping(ctx) }

func (t wsTransport) Close(reason string) error {
	return t.conn.Close(websocket.StatusNormalClosure, reason)
}
