// Package relayclient is the client half of the relay protocol, for a program
// that drives a fleet rather than being driven by one: the desktop window.
//
// One WebSocket carries every machine. Each machine the window looks at gets a
// logical *stream*, and inside that stream travels the identical session
// protocol a browser speaks to a daemon. The relay passes the payload through
// without reading it, so nothing here knows what a "session" or a "cursor" is —
// which is what lets the desktop reach another machine's shells under exactly
// the arbitration that machine's hub already performs.
//
// It is the Go shape of the browser client's relay.ts, and it exists for the
// same reason: a machine that can be taken over must also be able to take over.
package relayclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

// State is what the client's own link to the relay is doing.
type State string

const (
	StateAuth    State = "auth"
	StateOnline  State = "online"
	StateOffline State = "offline"
)

const (
	minBackoff  = 500 * time.Millisecond
	maxBackoff  = 10 * time.Second
	dialTimeout = 15 * time.Second
)

// Hooks are the client's own callbacks, given at construction: the roster and
// the link's state can change while Dial is still waiting for the welcome, so a
// callback assigned afterwards would miss the first of them.
type Hooks struct {
	// OnRoster is the machine list, on every change.
	OnRoster func([]tunnel.Machine)
	// OnState is the client's own link changing state.
	OnState func(State, string)
}

// Handlers is what one machine's stream reports.
type Handlers interface {
	// Open is the stream being routed: the machine's hub is reachable, and the
	// session protocol may begin.
	Open()
	// Frame is one session frame from that machine's hub.
	Frame(m protocol.Msg)
	// Close is the stream ending, with the reason.
	Close(reason string)
}

// Client is the desktop's connection to a relay as a client.
type Client struct {
	url        string
	token      string
	httpClient *http.Client

	// OnRoster is the roster as the relay pushes it, on every change.
	OnRoster func([]tunnel.Machine)
	// OnState is the client's own link changing state.
	OnState func(State, string)

	mu        sync.Mutex
	ws        *websocket.Conn
	authed    bool
	next      uint32
	byMachine map[string]*Stream
	byStream  map[uint32]*Stream
	machines  []tunnel.Machine
	closed    bool
	backoff   time.Duration
}

// Options configure a client's transport.
type Options struct {
	// CAFile is a PEM file holding the certificate that signed the relay's TLS
	// certificate, for a relay behind a private CA or a self-signed one. Empty
	// means the system roots. It is the same pin a member uses, and it is set
	// the same way: whoever set the fleet up points both at one file.
	CAFile string
}

// Dial connects to the relay, authenticates, and waits for the welcome. It
// returns once the client is online, or with the reason it could not be.
func Dial(ctx context.Context, url, token string, opts Options, hooks Hooks) (*Client, error) {
	httpClient := &http.Client{}
	if opts.CAFile != "" {
		pool, err := loadCA(opts.CAFile)
		if err != nil {
			return nil, err
		}
		httpClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}
	}
	c := &Client{
		url:        url,
		token:      token,
		httpClient: httpClient,
		OnRoster:   hooks.OnRoster,
		OnState:    hooks.OnState,
		byMachine:  map[string]*Stream{},
		byStream:   map[uint32]*Stream{},
		backoff:    minBackoff,
	}
	if err := c.connect(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// welcome is what the relay's first frame says: the token checked out, or it did
// not. Waiting for it is what makes a bad token fail fast rather than looking
// like a silently empty fleet.
type welcome struct {
	ok    bool
	error string
}

// loadCA reads a PEM certificate file into a root pool.
func loadCA(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("no certificate in " + path)
	}
	return pool, nil
}

// Machines is the roster as the relay last pushed it.
func (c *Client) Machines() []tunnel.Machine {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]tunnel.Machine(nil), c.machines...)
}

// Machine is one roster entry, or the zero value.
func (c *Client) Machine(id string) (tunnel.Machine, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.machines {
		if m.ID == id {
			return m, true
		}
	}
	return tunnel.Machine{}, false
}

// Open starts a stream to a machine, reporting through h. The stream lives
// while the machine is online and the client is connected: a machine that drops
// takes its streams with it, and a roster that brings it back re-opens them.
func (c *Client) Open(machine string, h Handlers) *Stream {
	c.mu.Lock()
	if existing := c.byMachine[machine]; existing != nil {
		existing.handlers = h
		c.mu.Unlock()
		c.flush()
		return existing
	}
	s := &Stream{client: c, machine: machine, handlers: h}
	c.byMachine[machine] = s
	c.mu.Unlock()
	c.flush()
	return s
}

// Close ends the client and every stream on it.
func (c *Client) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	ws := c.ws
	streams := make([]*Stream, 0, len(c.byMachine))
	for _, s := range c.byMachine {
		streams = append(streams, s)
	}
	c.byMachine = map[string]*Stream{}
	c.byStream = map[uint32]*Stream{}
	c.mu.Unlock()

	if ws != nil {
		_ = ws.Close(websocket.StatusNormalClosure, "client closing")
	}
	for _, s := range streams {
		s.end("client closing")
	}
}

// ---- transport ------------------------------------------------------------

func (c *Client) connect(ctx context.Context) error {
	c.setState(StateAuth, "")
	dctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	ws, _, err := websocket.Dial(dctx, c.url, &websocket.DialOptions{
		HTTPClient: c.httpClient,
	})
	if err != nil {
		c.retry("无法连接中继：" + err.Error())
		return err
	}

	c.mu.Lock()
	c.ws = ws
	c.mu.Unlock()

	// The token travels in the first frame, never in the URL: a query string
	// ends up in access logs, proxy logs and browser history.
	if !c.raw(tunnel.Frame{T: tunnel.TAuth, Token: c.token}) {
		return errors.New("relay: auth frame not sent")
	}

	// The first frame back is the answer about the token: welcome or refusal.
	// The read loop takes over from there.
	first := make(chan welcome, 1)
	go c.readLoop(ws, first)
	select {
	case w := <-first:
		if !w.ok {
			_ = ws.Close(websocket.StatusNormalClosure, "auth failed")
			return errors.New("relay: " + w.error)
		}
	case <-dctx.Done():
		_ = ws.CloseNow()
		return dctx.Err()
	case <-time.After(dialTimeout):
		_ = ws.CloseNow()
		return errors.New("relay: no answer about the token")
	}
	return nil
}

// readLoop decodes frames until the socket ends. first, when set, takes the
// answer about the token and is then nil: the loop carries on without it.
func (c *Client) readLoop(ws *websocket.Conn, first chan<- welcome) {
	for {
		_, b, err := ws.Read(context.Background())
		if err != nil {
			if first != nil {
				first <- welcome{error: "connection closed before the relay answered"}
			}
			c.linkDown(ws, "中继连接断开")
			return
		}
		var f tunnel.Frame
		if err := json.Unmarshal(b, &f); err != nil {
			continue
		}
		if first != nil {
			// The welcome is reported to Dial and then handled like any other
			// frame: it is what marks the client authed, and the roster push
			// that follows it must not be the thing that does.
			first <- welcome{ok: f.T == tunnel.TAuthOK, error: frameError(f)}
			first = nil
			if f.T != tunnel.TAuthOK {
				c.linkDown(ws, frameError(f))
				return
			}
		}
		c.handle(f)
	}
}

// frameError is a frame's reason, for the frames that carry one.
func frameError(f tunnel.Frame) string {
	if f.Message != "" {
		return f.Message
	}
	return f.Code
}

func (c *Client) linkDown(ws *websocket.Conn, reason string) {
	c.mu.Lock()
	if c.ws != ws {
		c.mu.Unlock()
		return // a newer connection already replaced this one
	}
	c.ws = nil
	c.authed = false
	streams := make([]*Stream, 0, len(c.byStream))
	for _, s := range c.byStream {
		streams = append(streams, s)
	}
	c.byStream = map[uint32]*Stream{}
	for _, s := range c.byMachine {
		s.stream = 0
		s.accepted = false
	}
	c.mu.Unlock()

	for _, s := range streams {
		s.end(reason)
	}
	if c.isClosed() {
		return
	}
	c.retry(reason)
}

func (c *Client) handle(f tunnel.Frame) {
	switch f.T {
	case tunnel.TAuthOK:
		c.mu.Lock()
		c.authed = true
		c.backoff = minBackoff
		c.mu.Unlock()
		c.setState(StateOnline, "")
		c.flush()
	case tunnel.TMachines:
		c.mu.Lock()
		c.machines = f.Machines
		c.mu.Unlock()
		if c.OnRoster != nil {
			c.OnRoster(append([]tunnel.Machine(nil), f.Machines...))
		}
		c.flush()
	case tunnel.TAccept:
		if s := c.streamOf(f.Stream); s != nil {
			c.mu.Lock()
			s.accepted = true
			c.mu.Unlock()
			s.handlers.Open()
		}
	case tunnel.TData:
		if s := c.streamOf(f.Stream); s != nil {
			var m protocol.Msg
			if err := json.Unmarshal(f.Payload, &m); err == nil {
				s.handlers.Frame(m)
			}
		}
	case tunnel.TClose:
		if s := c.streamOf(f.Stream); s != nil {
			s.end("machine closed the stream")
		}
	case tunnel.TError:
		if s := c.streamOf(f.Stream); s != nil {
			s.end(f.Message)
		} else {
			c.setState(StateOffline, f.Message)
		}
	}
}

// flush opens a stream for every online machine that has none, and tears down
// the ones whose machine has gone. The roster drives it, so a machine coming
// back is picked up without the client guessing at retry timing.
func (c *Client) flush() {
	c.mu.Lock()
	if !c.authed {
		c.mu.Unlock()
		return
	}
	online := map[string]bool{}
	for _, m := range c.machines {
		if m.Online {
			online[m.ID] = true
		}
	}
	var opening []*Stream
	var closing []*Stream
	for id, s := range c.byMachine {
		switch {
		case online[id] && s.stream == 0:
			c.next++
			s.stream = c.next
			c.byStream[s.stream] = s
			opening = append(opening, s)
		case !online[id] && s.stream != 0:
			delete(c.byStream, s.stream)
			s.stream, s.accepted = 0, false
			closing = append(closing, s)
		}
	}
	c.mu.Unlock()

	for _, s := range opening {
		c.raw(tunnel.Frame{T: tunnel.TOpen, Machine: s.machine, Stream: s.stream})
	}
	for _, s := range closing {
		s.end("machine offline")
	}
}

func (c *Client) streamOf(id uint32) *Stream {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byStream[id]
}

func (c *Client) raw(f tunnel.Frame) bool {
	c.mu.Lock()
	ws := c.ws
	c.mu.Unlock()
	if ws == nil {
		return false
	}
	b, err := json.Marshal(f)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return ws.Write(ctx, websocket.MessageText, b) == nil
}

func (c *Client) setState(s State, detail string) {
	if c.OnState != nil {
		c.OnState(s, detail)
	}
}

func (c *Client) retry(detail string) {
	c.setState(StateOffline, detail)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	delay := c.backoff
	c.backoff = min(c.backoff*2, maxBackoff)
	c.mu.Unlock()

	go func() {
		// Half the backoff, plus up to half again: a fleet whose members all
		// drop at once should not all come back in lockstep.
		jitter := time.Duration(rand.Int63n(int64(delay)/2 + 1))
		time.Sleep(delay/2 + jitter)
		if c.isClosed() {
			return
		}
		_ = c.connect(context.Background())
	}()
}

func (c *Client) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// Stream is one machine's conversation: the session protocol carried inside it.
type Stream struct {
	client   *Client
	machine  string
	handlers Handlers

	mu       sync.Mutex
	stream   uint32
	accepted bool
	closed   bool
}

// Send puts one session frame on the stream. It reports whether the frame went
// out; a frame that did not is one the caller still holds.
func (s *Stream) Send(m protocol.Msg) bool {
	s.mu.Lock()
	stream, ok := s.stream, s.accepted && !s.closed
	s.mu.Unlock()
	if !ok {
		return false
	}
	payload, err := json.Marshal(m)
	if err != nil {
		return false
	}
	return s.client.raw(tunnel.Frame{T: tunnel.TData, Stream: stream, Payload: payload})
}

// Close ends the stream. The machine's shells are left alone.
func (s *Stream) Close() {
	s.mu.Lock()
	stream, accepted := s.stream, s.accepted
	s.stream, s.accepted, s.closed = 0, false, true
	s.mu.Unlock()
	if stream != 0 && accepted {
		s.client.raw(tunnel.Frame{T: tunnel.TClose, Stream: stream})
	}
	s.client.forget(s)
}

// end is the stream being finished from the client's side: the socket went
// away, or the machine dropped.
func (s *Stream) end(reason string) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.stream, s.accepted = 0, false
	s.mu.Unlock()
	s.handlers.Close(reason)
}

// forget drops a stream from the client's tables.
func (c *Client) forget(s *Stream) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cur := c.byMachine[s.machine]; cur == s {
		delete(c.byMachine, s.machine)
	}
	if s.stream != 0 {
		delete(c.byStream, s.stream)
	}
}

// hostOf is the relay's host name, for a panel that lists machines by name.
func hostOf(url string) string {
	return strings.TrimPrefix(strings.TrimPrefix(url, "wss://"), "ws://")
}
