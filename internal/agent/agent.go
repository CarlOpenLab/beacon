// Package agent is the member half of a relay link.
//
// It dials *out* to a relay and keeps one long-lived tunnel open. When a
// remote client opens a stream, the agent hands a hub an in-process transport
// for it, so the hub attaches that viewer through exactly the code path a
// local browser takes. Nothing in internal/hub knows the relay exists.
//
// The dial-out direction is the whole point: a machine behind NAT or CGNAT
// needs no port forward, no public address, and no inbound firewall rule.
package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/CarlOpenLab/term-fleet/internal/diag"
	"github.com/CarlOpenLab/term-fleet/internal/hub"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

const (
	helloTimeout  = 15 * time.Second
	writeTimeout  = 10 * time.Second
	pingInterval  = 30 * time.Second
	pingTimeout   = 10 * time.Second
	outboundQueue = 256
	// A link that survives this long is considered healthy, so the next
	// reconnect starts from the short backoff again instead of the cap.
	healthyLink = 30 * time.Second
)

// Config is everything an agent needs.
type Config struct {
	// URL is the relay's member endpoint, e.g. wss://relay.example/relay/agent.
	URL string
	// DeviceID and Secret are this member's credential, issued when the relay
	// was configured. Never a shared fleet secret: this one identifies a single
	// machine, so it can be revoked on its own.
	DeviceID string
	Secret   string
	// MachineName is what a person sees in the machine list.
	MachineName string
	// RootCAs, when set, replaces the system roots for the relay's TLS
	// certificate. Relays are often fronted by a private CA (or a self-signed
	// certificate); pinning it here keeps a member connected without asking
	// every machine to install it system-wide.
	RootCAs *x509.CertPool
	// Hub is the local session authority. The agent reads and writes session
	// state only through it.
	Hub *hub.Hub

	Log *log.Logger
	// OnState, when set, is called whenever the link to the relay changes
	// state. It exists so a window can say why a machine is not reachable
	// instead of leaving a switch on and nothing else to go on. It is called
	// from the agent's own goroutine and must not block.
	OnState func(State)
	// Backoff tuning. Tests set these small; production uses the defaults.
	MinBackoff time.Duration
	MaxBackoff time.Duration

	// Diag, when set, is the diagnostic log this machine pushes to the relay
	// beside the tunnel. The admin panel reads the newest tail, which is the one
	// piece of a bug report a person on another machine cannot get any other
	// way: the daemon often runs with no console at all.
	Diag *diag.Logger
	// LogURL overrides the address the log is posted to. Empty derives it from
	// URL (the /agent endpoint's sibling /log), which is what production uses.
	LogURL string
	// LogInterval is how often the tail is pushed. Zero means every 30s.
	LogInterval time.Duration
}

// State is how the link to the relay is doing, in the terms a person needs in
// order to tell "not set up" from "set up and failing".
type State struct {
	// Connecting is true while an attempt is in flight.
	Connecting bool
	// Connected is true once the relay has welcomed this member.
	Connected bool
	// Detail is a one-line summary: the address being tried, or why the last
	// attempt ended.
	Detail string
}

func (c Config) withDefaults() Config {
	if c.Log == nil {
		c.Log = log.Default()
	}
	if c.MinBackoff <= 0 {
		c.MinBackoff = 500 * time.Millisecond
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 30 * time.Second
	}
	if c.LogInterval <= 0 {
		c.LogInterval = 30 * time.Second
	}
	if c.MachineName == "" {
		if h, err := os.Hostname(); err == nil {
			c.MachineName = h
		} else {
			c.MachineName = c.DeviceID
		}
	}
	return c
}

// LoadCACert reads a PEM certificate file and returns it as a root pool. It is
// how a member trusts a relay whose certificate was not issued by a public CA
// (a private CA, or a self-signed certificate). The file may hold a CA
// certificate or the self-signed leaf itself; either way it becomes the only
// root the dial will accept, so a wrong certificate fails closed instead of
// quietly falling back to the system store.
func LoadCACert(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("agent: %s: no PEM certificate found", path)
	}
	return pool, nil
}

// Agent keeps a member connected to a relay.
type Agent struct {
	cfg Config
	log *log.Logger
}

// New builds an agent.
func New(cfg Config) (*Agent, error) {
	if cfg.URL == "" {
		return nil, errors.New("agent: no relay URL")
	}
	if u, err := url.Parse(cfg.URL); err != nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" {
		return nil, fmt.Errorf("agent: relay URL must be ws:// or wss://, got %q", cfg.URL)
	}
	if cfg.DeviceID == "" || cfg.Secret == "" {
		return nil, errors.New("agent: device id and secret are required")
	}
	if cfg.Hub == nil {
		return nil, errors.New("agent: no hub")
	}
	cfg = cfg.withDefaults()
	return &Agent{cfg: cfg, log: cfg.Log}, nil
}

// Run dials the relay and keeps it dialed until ctx is cancelled. A dropped
// link is retried with exponential backoff; the hub keeps running throughout,
// so sessions outlive a network outage without their output being lost (the
// per-session ring still holds it, and a client re-attaching with its cursor
// gets only the gap).
func (a *Agent) Run(ctx context.Context) error {
	if a.cfg.Diag != nil {
		go a.reportLogs(ctx)
	}
	backoff := a.cfg.MinBackoff
	for {
		start := time.Now()
		a.report(State{Connecting: true, Detail: "正在连接 " + a.cfg.URL})
		err := a.serveOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Since(start) > healthyLink {
			backoff = a.cfg.MinBackoff
		}
		a.log.Printf("agent: relay link ended after %s: %v", time.Since(start).Round(time.Millisecond), err)
		a.report(State{Detail: shortReason(err)})
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < a.cfg.MaxBackoff {
			backoff *= 2
			if backoff > a.cfg.MaxBackoff {
				backoff = a.cfg.MaxBackoff
			}
		}
	}
}

// report hands a state change to whoever is watching, if anyone is.
func (a *Agent) report(s State) {
	if a.cfg.OnState != nil {
		a.cfg.OnState(s)
	}
}

// shortReason turns a dial or handshake error into something that fits in a
// settings window. The certificate failure in particular is worth naming: it is
// what a missing or wrong CA looks like from the outside, and "machine stays
// offline" is not a useful thing to hand someone.
func shortReason(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "certificate"):
		return "连不上：证书校验失败，检查「CA 证书」是否填对 —— " + msg
	case strings.Contains(msg, "refused the link"):
		return "连不上：中继拒绝了这组设备 ID / 密钥 —— " + msg
	case strings.Contains(msg, "connection refused"), strings.Contains(msg, "no such host"),
		strings.Contains(msg, "timeout"), strings.Contains(msg, "i/o timeout"):
		return "连不上：网络不可达 —— " + msg
	default:
		return "连不上：" + msg
	}
}

// serveOnce holds one tunnel open until it fails.
func (a *Agent) serveOnce(ctx context.Context) error {
	host, _ := os.Hostname()
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+a.cfg.DeviceID+":"+a.cfg.Secret)

	dctx, cancel := context.WithTimeout(ctx, helloTimeout)
	dialOpts := &websocket.DialOptions{HTTPHeader: hdr}
	if a.cfg.RootCAs != nil {
		dialOpts.HTTPClient = &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: a.cfg.RootCAs}},
		}
	}
	conn, _, err := websocket.Dial(dctx, a.cfg.URL, dialOpts)
	cancel()
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	// The relay may hand back a session's whole ring on a re-attach, and this
	// socket must be able to read it: the library's 32 KiB default would close
	// the link with StatusMessageTooBig instead of delivering the bytes.
	conn.SetReadLimit(protocol.MaxMessageBytes)
	defer conn.CloseNow()

	s := &session{
		a:       a,
		conn:    conn,
		out:     make(chan tunnel.Frame, outboundQueue),
		done:    make(chan struct{}),
		streams: map[uint32]*streamTransport{},
	}

	// Say hello and wait to be welcomed. Registering only after the relay has
	// acknowledged means a rejected credential fails fast instead of looking
	// like a silent, permanently empty fleet.
	// What this machine says about itself comes from its hub, not from the
	// process that dialled: the hub is what a client will be talking to, so its
	// version and instance are the ones that belong on the roster.
	hostInfo := a.cfg.Hub.Host()
	if hostInfo.Hostname == "" {
		hostInfo.Hostname = host
	}
	if !s.send(tunnel.Frame{
		T:           tunnel.TAgentHello,
		V:           tunnel.Version,
		MachineID:   a.cfg.DeviceID,
		MachineName: a.cfg.MachineName,
		Host:        hostInfo,
	}) {
		return errors.New("handshake send failed")
	}

	go s.writeLoop(ctx)

	hctx, hcancel := context.WithTimeout(ctx, helloTimeout)
	welcome, err := readFrame(hctx, conn)
	hcancel()
	if err != nil {
		return fmt.Errorf("awaiting welcome: %w", err)
	}
	if welcome.T == tunnel.TError {
		return fmt.Errorf("relay refused the link: %s (%s)", welcome.Message, welcome.Code)
	}
	if welcome.T != tunnel.TAgentWelcome {
		return fmt.Errorf("unexpected frame %q while awaiting welcome", welcome.T)
	}
	a.log.Printf("agent: connected to relay as %q", a.cfg.DeviceID)
	a.report(State{Connected: true, Detail: "已接入中继，设备 ID " + a.cfg.DeviceID})

	defer func() {
		s.closeAll()
		close(s.done)
		_ = conn.Close(websocket.StatusNormalClosure, "link ended")
	}()

	for {
		f, err := readFrame(ctx, conn)
		if err != nil {
			return err
		}
		switch f.T {
		case tunnel.TOpen:
			s.openStream(ctx, f.Stream)
		case tunnel.TData:
			s.deliver(f.Stream, f.Payload)
		case tunnel.TClose:
			s.closeStream(f.Stream)
		default:
			// Routing is the relay's business; unknown frames are ignored.
		}
	}
}

// ---------------------------------------------------------------- session

type session struct {
	a    *Agent
	conn *websocket.Conn
	out  chan tunnel.Frame
	done chan struct{}
	once sync.Once

	mu      sync.Mutex
	streams map[uint32]*streamTransport
	// blocked counts frames that found the out queue full, so the warning below
	// can be said once and then summarised instead of one line per frame of a
	// busy shell.
	blocked int
}

func (s *session) send(f tunnel.Frame) bool {
	select {
	case <-s.done:
		return false
	case s.out <- f:
		return true
	default:
		s.noteBlocked(f)
		return false
	}
}

// noteBlocked says that the queue a machine shares between its viewers is full.
// It is the line that explains a remote viewer whose replay was late or lost:
// the machine was producing faster than the relay could take it.
func (s *session) noteBlocked(f tunnel.Frame) {
	s.mu.Lock()
	s.blocked++
	n := s.blocked
	s.mu.Unlock()
	if n == 1 || n%100 == 0 {
		diag.Warnf("relay", "agent queue full: frame dropped type=%s stream=%d count=%d", f.T, f.Stream, n)
	}
}

// sendCtx is send that waits for room, up to ctx. It is what a viewer's output
// uses: the out queue is shared by every viewer of this machine, and a full one
// must slow the hub's writer down rather than answer it with an error the hub
// reads as a dead client — which killed the viewer mid-replay and left its tab
// a blank pane with a live shell behind it.
func (s *session) sendCtx(ctx context.Context, f tunnel.Frame) error {
	// The fast path: room was there, so the frame is on its way and nothing is
	// worth saying.
	select {
	case <-s.done:
		return errors.New("relay link unavailable")
	case s.out <- f:
		return nil
	default:
	}
	// Backpressure: the queue is full and this frame waits for room rather than
	// being answered with an error the hub would read as a dead client. Saying
	// so once per hundred waits separates "the link is slow" from "the viewer
	// never attached", which look identical from the far side.
	s.noteBlocked(f)
	select {
	case <-s.done:
		return errors.New("relay link unavailable")
	case <-ctx.Done():
		return ctx.Err()
	case s.out <- f:
		return nil
	}
}

// openStream projects one remote viewer onto the local hub. Each stream gets
// its own hub client, so a phone watching three sessions is three viewers —
// which is exactly what the control arbitration already expects.
func (s *session) openStream(ctx context.Context, id uint32) {
	tp := &streamTransport{
		in:   make(chan []byte, inboundQueue),
		done: make(chan struct{}),
	}
	s.mu.Lock()
	s.streams[id] = tp
	s.mu.Unlock()

	tp.send = func(ctx context.Context, payload []byte) error {
		return s.sendCtx(ctx, tunnel.Frame{T: tunnel.TData, Stream: id, Payload: payload})
	}

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.streams, id)
			s.mu.Unlock()
			s.send(tunnel.Frame{T: tunnel.TClose, Stream: id})
		}()
		s.a.cfg.Hub.ServeTransport(tp)
	}()
}

func (s *session) deliver(id uint32, payload []byte) {
	s.mu.Lock()
	tp := s.streams[id]
	s.mu.Unlock()
	if tp == nil {
		return
	}
	tp.push(payload)
}

func (s *session) closeStream(id uint32) {
	s.mu.Lock()
	tp := s.streams[id]
	delete(s.streams, id)
	s.mu.Unlock()
	if tp != nil {
		tp.Close("stream closed by relay")
	}
}

func (s *session) closeAll() {
	s.mu.Lock()
	all := make([]*streamTransport, 0, len(s.streams))
	for _, tp := range s.streams {
		all = append(all, tp)
	}
	s.streams = map[uint32]*streamTransport{}
	s.mu.Unlock()
	for _, tp := range all {
		tp.Close("relay link lost")
	}
}

func (s *session) writeLoop(ctx context.Context) {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ctx.Done():
			return
		case f := <-s.out:
			b, err := json.Marshal(f)
			if err != nil {
				continue
			}
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err = s.conn.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, pingTimeout)
			err := s.conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// ---------------------------------------------------------------- transport

// inboundQueue bounds how far the relay may run ahead of a hub that is slower
// to drain than the network is to deliver.
const inboundQueue = 512

// streamTransport is a hub.Transport backed by tunnel frames rather than a
// socket.
type streamTransport struct {
	in   chan []byte
	send func(ctx context.Context, payload []byte) error
	done chan struct{}
	once sync.Once
}

func (s *streamTransport) push(b []byte) {
	select {
	case <-s.done:
	case s.in <- b:
	default:
		// The hub is not draining. Dropping the viewer is better than growing
		// an unbounded queue, and the hub will detach it cleanly.
		s.Close("viewer too slow")
	}
}

func (s *streamTransport) Read(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, io.EOF
	case b := <-s.in:
		return b, nil
	}
}

func (s *streamTransport) Write(ctx context.Context, b []byte) error {
	return s.send(ctx, b)
}

// Ping is a no-op: the relay link has its own heartbeat, and a session's
// liveness is already implied by the tunnel being up.
func (s *streamTransport) Ping(ctx context.Context) error { return nil }

func (s *streamTransport) Close(reason string) error {
	s.once.Do(func() { close(s.done) })
	return nil
}

// ---------------------------------------------------------------- helpers

func readFrame(ctx context.Context, conn *websocket.Conn) (tunnel.Frame, error) {
	var f tunnel.Frame
	_, data, err := conn.Read(ctx)
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return f, err
	}
	return f, nil
}
