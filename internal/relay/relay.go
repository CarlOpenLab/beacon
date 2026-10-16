// Package relay is the public-side router of a term-fleet.
//
// Members (anything running a hub) dial *out* to it, so a machine behind NAT
// needs no port forward and no public address. Clients connect to the same
// relay from anywhere. The relay pairs the two and forwards bytes.
//
// It is deliberately not a brain: it never parses an inner frame, never holds
// a PTY, and never decides who controls a session. A member that disconnects
// takes its sessions' authority with it, and each hub keeps arbitrating
// exactly as it does on a LAN. That is what keeps a future protocol feature
// from needing a relay release.
package relay

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

const (
	writeTimeout  = 10 * time.Second
	pingInterval  = 30 * time.Second
	pingTimeout   = 10 * time.Second
	outboundQueue = 256
	// maxStreamsPerClient bounds how much one client can make members open.
	maxStreamsPerClient = 64
)

// Config is the relay's whole configuration.
type Config struct {
	// Devices maps a member id to its secret. A member authenticates with
	// "Authorization: Bearer <id>:<secret>". Per-member credentials rather than
	// one shared secret: revoking a laptop must not require re-enrolling the
	// fleet, and a leaked member secret must not expose another member's
	// screens.
	//
	// Used only when Registry is nil; it is the fixed-map form, kept for tests
	// and for a relay driven entirely from flags. The relay proper serves out of
	// Registry, because machines are enrolled while it runs.
	Devices map[string]string
	// Registry is the live member registry. Nil means "wrap Devices in an
	// in-memory registry that is never written anywhere".
	Registry *Registry
	// ClientToken is the shared secret a client presents. Browsers cannot set
	// headers on a WebSocket, so it also arrives in the first frame.
	ClientToken string
	// ControlTokenTTL is how long a credential minted for an enrolled machine
	// stays valid. Zero means DefaultControlTokenTTL.
	ControlTokenTTL time.Duration
	// Token is the live client-token store, so the admin page can rotate the
	// token without a redeploy. Nil means "wrap ClientToken in an in-memory
	// store". When set, it takes precedence over ClientToken.
	Token          *TokenStore
	OriginPatterns []string
	// TrustProxy makes the relay read X-Forwarded-For for the limiter's source
	// address. Only enable it behind a proxy you control: otherwise a caller can
	// forge the header and pretend to be a new address on every attempt.
	TrustProxy bool
	// AuthMaxFailures is how many rejected attempts one address may make inside
	// AuthWindow before it is locked out for AuthCooldown. Zero disables the
	// limiter, which is only reasonable on a loopback-only relay.
	AuthMaxFailures int
	AuthWindow      time.Duration
	AuthCooldown    time.Duration
	Log             *log.Logger
}

// Relay routes streams between members and clients.
type Relay struct {
	cfg Config
	log *log.Logger
	lim *authLimiter
	// reg and token are the parts of the configuration that can change while the
	// relay runs. Everything else in cfg is read-only after New.
	reg   *Registry
	token *TokenStore

	mu         sync.Mutex
	members    map[string]*member
	nextStream uint32

	rosterMu sync.Mutex
	roster   map[*clientConn]struct{}
	seq      int
}

// New builds a relay. A client token must be set: a relay without authentication
// is a remote shell for whoever finds the port, and by definition it is reachable
// from the open internet.
//
// An *empty* member registry is allowed. It is the state of a freshly deployed
// relay whose operator has not enrolled a machine yet, and the admin page is the
// intended way to leave that state. No member can dial in until one exists, so
// the empty registry is inert rather than dangerous; the client token is what
// keeps the port closed, and it is still required.
func New(cfg Config) (*Relay, error) {
	if cfg.Registry == nil {
		cfg.Registry = inMemoryRegistry(cfg.Devices)
	}
	if cfg.Token == nil {
		cfg.Token = NewTokenStore(cfg.ClientToken, "")
	}
	if cfg.Token.Get() == "" {
		return nil, errors.New("relay: no client token configured")
	}
	if cfg.Log == nil {
		cfg.Log = log.Default()
	}
	// Defaults matter here: a relay reachable from the internet with no limiter
	// at all is the one configuration that must not be reachable by accident.
	if cfg.AuthMaxFailures == 0 {
		cfg.AuthMaxFailures = 10
	}
	if cfg.AuthWindow == 0 {
		cfg.AuthWindow = time.Minute
	}
	if cfg.AuthCooldown == 0 {
		cfg.AuthCooldown = 5 * time.Minute
	}
	return &Relay{
		cfg:     cfg,
		log:     cfg.Log,
		lim:     newAuthLimiter(cfg.AuthMaxFailures, cfg.AuthWindow, cfg.AuthCooldown, cfg.TrustProxy),
		reg:     cfg.Registry,
		token:   cfg.Token,
		members: map[string]*member{},
		roster:  map[*clientConn]struct{}{},
	}, nil
}

// Devices exposes the live registry, for the admin page to enroll and revoke.
func (r *Relay) Devices() *Registry { return r.reg }

// ClientToken exposes the live token store, for the admin page to rotate.
func (r *Relay) ClientToken() *TokenStore { return r.token }

// Roster returns the current machine list as clients see it.
func (r *Relay) Roster() []tunnel.Machine { return r.snapshot() }

// RefreshRoster re-broadcasts the machine list. Call it after the registry
// changes so connected browsers learn about a newly enrolled machine, and stop
// offering one that was just revoked, without reconnecting.
func (r *Relay) RefreshRoster() { r.broadcastMachines() }

// RevokeDevice removes a machine's enrollment and every credential minted to it.
// A revoked machine has to lose its reach now rather than when its credentials
// happen to expire: the registry is what a member dials against, and a minted
// token is what it drives the rest of the fleet with.
func (r *Relay) RevokeDevice(id string) (bool, error) {
	removed, err := r.reg.Revoke(id)
	if err != nil {
		return removed, err
	}
	if n := r.token.ForgetDevice(id); n > 0 {
		r.log.Printf("relay: forgot %d minted credential(s) of %s", n, id)
	}
	return removed, nil
}

// ---------------------------------------------------------------- registry

// Online reports how many members are connected right now. It exists for health
// checks: a relay with zero online members is up but useless, and that is worth
// being able to see from outside.
func (r *Relay) Online() int {
	n := 0
	for _, m := range r.snapshot() {
		if m.Online {
			n++
		}
	}
	return n
}

func (r *Relay) snapshot() []tunnel.Machine {
	r.mu.Lock()
	members := make(map[string]*member, len(r.members))
	for id, m := range r.members {
		members[id] = m
	}
	r.mu.Unlock()

	entries := r.reg.Entries()
	out := make([]tunnel.Machine, 0, len(entries))
	for _, e := range entries {
		m := tunnel.Machine{ID: e.ID}
		id := e.ID
		if live, ok := members[id]; ok {
			m.Name = live.name
			m.Host = live.host
			m.Online = true
		}
		// The enrolled name is the authoritative label: a member that renamed
		// itself off-line should still be recognizable in the list.
		if m.Name == "" {
			m.Name = e.Name
		}
		out = append(out, m)
	}
	// A stable order: the client renders the list as sent, and a list that
	// reshuffles on every presence change would make the phone and the PC
	// disagree about which row is which.
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Relay) memberByID(id string) *member {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.members[id]
}

func (r *Relay) allocStream() uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextStream++
	if r.nextStream == 0 { // uint32 wrap; 0 is never a valid stream id
		r.nextStream++
	}
	return r.nextStream
}

// ---------------------------------------------------------------- auth

func (r *Relay) agentAuthOK(rq *http.Request) (string, bool) {
	raw, ok := strings.CutPrefix(rq.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return "", false
	}
	id, secret, ok := strings.Cut(raw, ":")
	if !ok {
		return "", false
	}
	want, known := r.reg.Secret(id)
	if !known {
		return "", false
	}
	if subtle.ConstantTimeCompare([]byte(secret), []byte(want)) != 1 {
		return "", false
	}
	return id, true
}

// tokenOK reports whether a presented client credential is one this relay
// honours right now: the shared token while it is within its lifetime, or a live
// credential minted for an enrolled machine. Every path that admits a viewer
// funnels through here, so an expired token stops working everywhere at once.
func (r *Relay) tokenOK(got string) bool {
	return r.token.Accept(got)
}

// ControlToken mints a short-lived client credential for an enrolled machine.
//
// It is how one desktop reaches the rest of the fleet without anyone typing a
// token at a settings screen: the machine proves it is itself with the device
// credential it already dials the member link with, and gets back a credential
// it may act as a client with for a few minutes. The shared token stays what it
// was — what a browser or a phone is given — so handing out reach does not hand
// out the fleet's one secret, and revoking the machine takes this away with it.
func (r *Relay) ControlToken(w http.ResponseWriter, rq *http.Request) {
	if rq.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	ip := clientIP(rq, r.cfg.TrustProxy)
	if allowed, wait := r.lim.allow(ip); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		http.Error(w, "too many failed attempts", http.StatusTooManyRequests)
		return
	}
	id, ok := r.agentAuthOK(rq)
	if !ok {
		r.lim.fail(ip)
		r.log.Printf("relay: refused a control token to %s: not an enrolled machine", ip)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	r.lim.ok(ip)

	ttl := r.cfg.ControlTokenTTL
	if ttl <= 0 {
		ttl = DefaultControlTokenTTL
	}
	token, expiresAt, err := r.token.Mint(id, ttl)
	if err != nil {
		r.log.Printf("relay: minting a control token for %s: %v", id, err)
		http.Error(w, "cannot mint a token", http.StatusInternalServerError)
		return
	}
	r.log.Printf("relay: minted a control token for %s, good for %s", id, ttl.Round(time.Second))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":     token,
		"expiresAt": expiresAt.UTC().Format(time.RFC3339),
		"device":    id,
	})
}

// headerToken reads a client credential from an Authorization header. Usable by
// ordinary HTTP clients and by anything that can set WebSocket headers; a
// browser cannot, which is why the socket path also accepts a first frame.
func (r *Relay) headerToken(rq *http.Request) string {
	return strings.TrimPrefix(rq.Header.Get("Authorization"), "Bearer ")
}

// authenticateClient enforces the credential on a freshly accepted socket.
//
// A valid Authorization header short-circuits it. Otherwise the very first
// frame must be an auth frame; anything else — including silence past the
// deadline — closes the socket. The token never travels in the URL, so it
// cannot leak through access logs or browser history.
func (r *Relay) authenticateClient(ctx context.Context, conn *websocket.Conn) (bool, string) {
	authCtx, cancel := context.WithTimeout(ctx, tunnel.AuthTimeoutMS*time.Millisecond)
	defer cancel()
	f, err := readFrame(authCtx, conn)
	if err != nil {
		return false, "no credential"
	}
	if f.T != tunnel.TAuth {
		return false, "expected auth frame, got " + f.T
	}
	if !r.tokenOK(f.Token) {
		return false, "bad token"
	}
	return true, ""
}

// ---------------------------------------------------------------- endpoints

// ServeAgent handles a member dialing out: authenticate, await its hello,
// register it, then route until the link drops.
func (r *Relay) ServeAgent(w http.ResponseWriter, rq *http.Request) {
	// A member secret guards a whole machine, so it gets the same treatment as
	// the client token: guessing is rate-limited from both directions.
	ip := clientIP(rq, r.cfg.TrustProxy)
	if allowed, wait := r.lim.allow(ip); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		http.Error(w, "too many failed attempts", http.StatusTooManyRequests)
		return
	}
	id, ok := r.agentAuthOK(rq)
	if !ok {
		r.lim.fail(ip)
		r.log.Printf("relay: rejected member from %s", ip)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	r.lim.ok(ip)
	conn, err := websocket.Accept(w, rq, &websocket.AcceptOptions{OriginPatterns: r.cfg.OriginPatterns})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	ctx := rq.Context()

	// The first frame must be the hello; nothing else is accepted before it.
	hello, err := readFrame(ctx, conn)
	if err != nil || hello.T != tunnel.TAgentHello {
		_ = writeFrame(ctx, conn, tunnel.Frame{T: tunnel.TError, Code: tunnel.ErrBadRequest,
			Message: "expected agent.hello"})
		return
	}
	if hello.V != tunnel.Version {
		_ = writeFrame(ctx, conn, tunnel.Frame{T: tunnel.TError, Code: tunnel.ErrBadRequest,
			Message: "unsupported tunnel version"})
		return
	}

	m := &member{
		r:       r,
		id:      id,
		name:    hello.MachineName,
		host:    hello.Host,
		conn:    conn,
		out:     make(chan tunnel.Frame, outboundQueue),
		done:    make(chan struct{}),
		streams: map[uint32]*streamRef{},
	}

	// Replace a stale link for the same member: a reconnect can overlap the old
	// socket before the relay has noticed it died.
	r.mu.Lock()
	old := r.members[id]
	r.members[id] = m
	r.mu.Unlock()
	if old != nil {
		r.log.Printf("relay: member %s reconnected; dropping the old link", id)
		old.close("replaced by a newer link")
	}

	r.log.Printf("relay: member %s online (%s/%s)", id, hello.Host.Platform, hello.Host.Arch)
	r.broadcastMachines()
	defer func() {
		r.mu.Lock()
		if r.members[id] == m {
			delete(r.members, id)
		}
		r.mu.Unlock()
		m.close("member disconnected")
		r.log.Printf("relay: member %s offline", id)
		r.broadcastMachines()
	}()

	if !m.send(tunnel.Frame{T: tunnel.TAgentWelcome, V: tunnel.Version, MachineID: id}) {
		return
	}

	go r.writeLoop(ctx, conn, m.out, m.done, func() { m.close("write failed") })

	for {
		f, err := readFrame(ctx, conn)
		if err != nil {
			return
		}
		switch f.T {
		case tunnel.TData:
			m.forwardToClient(f)
		case tunnel.TClose:
			m.forwardToClient(f)
			m.dropStream(f.Stream)
		case tunnel.TAgentHello:
			// A repeated hello is harmless; ignore it.
		default:
			// The relay acts on routing, not on inner semantics. Anything else
			// is a protocol mistake, but not worth killing a working link over.
		}
	}
}

// ServeClient handles a browser or app: authenticate, send the machine list,
// then route streams.
func (r *Relay) ServeClient(w http.ResponseWriter, rq *http.Request) {
	ip := clientIP(rq, r.cfg.TrustProxy)
	if allowed, wait := r.lim.allow(ip); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		http.Error(w, "too many failed attempts", http.StatusTooManyRequests)
		return
	}

	// Non-browser clients (tools, tests, native apps) may present the token as
	// a header. A header that is *present but wrong* is refused before the
	// upgrade, so a caller gets an honest 401 instead of an accepted socket that
	// closes a moment later. Browsers cannot set headers, so they authenticate
	// in the first frame instead.
	header := r.headerToken(rq)
	if header != "" && !r.tokenOK(header) {
		r.lim.fail(ip)
		r.log.Printf("relay: rejected client from %s: bad header token", ip)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := websocket.Accept(w, rq, &websocket.AcceptOptions{OriginPatterns: r.cfg.OriginPatterns})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	ctx := rq.Context()
	viaFrame := false
	if header == "" {
		ok, why := r.authenticateClient(ctx, conn)
		if !ok {
			r.lim.fail(ip)
			r.log.Printf("relay: rejected client from %s: %s", ip, why)
			_ = writeFrame(ctx, conn, errFrame(tunnel.ErrUnauthorized, "authentication required", 0))
			_ = conn.Close(websocket.StatusPolicyViolation, "unauthorized")
			return
		}
		viaFrame = true
	}
	r.lim.ok(ip)

	c := &clientConn{
		r:       r,
		conn:    conn,
		out:     make(chan tunnel.Frame, outboundQueue),
		done:    make(chan struct{}),
		streams: map[uint32]*stream{},
	}

	r.rosterMu.Lock()
	r.seq++
	n := r.seq
	r.rosterMu.Unlock()
	r.log.Printf("relay: client %d connected", n)

	defer func() {
		c.close("client disconnected")
		r.rosterMu.Lock()
		delete(r.roster, c)
		r.rosterMu.Unlock()
		r.log.Printf("relay: client %d disconnected", n)
	}()

	// Tell a frame-authenticated client it is in before the roster, so it can
	// distinguish "still authenticating" from "authenticated, awaiting data".
	// A header-authenticated caller was already told at the HTTP layer.
	//
	// This has to precede joining the roster, because joining is what lets a
	// concurrent broadcastMachines reach this connection: a member joining at
	// the wrong moment would otherwise push a roster frame ahead of auth.ok and
	// leave the client unable to tell the two states apart.
	if viaFrame {
		c.send(tunnel.Frame{T: tunnel.TAuthOK})
	}

	r.rosterMu.Lock()
	r.roster[c] = struct{}{}
	r.rosterMu.Unlock()

	c.send(tunnel.Frame{T: tunnel.TMachines, Machines: r.snapshot()})

	go r.writeLoop(ctx, conn, c.out, c.done, func() { c.close("write failed") })

	for {
		f, err := readFrame(ctx, conn)
		if err != nil {
			return
		}
		switch f.T {
		case tunnel.TOpen:
			r.handleOpen(c, f)
		case tunnel.TData:
			r.routeFromClient(c, f)
		case tunnel.TClose:
			r.handleClientClose(c, f)
		default:
			c.send(errFrame(tunnel.ErrBadRequest, "unexpected frame "+f.T, f.Stream))
		}
	}
}

// handleOpen wires one client stream to one member stream.
func (r *Relay) handleOpen(c *clientConn, f tunnel.Frame) {
	if f.Machine == "" || f.Stream == 0 {
		c.send(errFrame(tunnel.ErrBadRequest, "open needs a machine and a nonzero stream", f.Stream))
		return
	}
	m := r.memberByID(f.Machine)
	if m == nil {
		c.send(errFrame(tunnel.ErrMachineOffline, "machine "+f.Machine+" is offline", f.Stream))
		return
	}
	if c.streamCount() >= maxStreamsPerClient {
		c.send(errFrame(tunnel.ErrStreamLimit, "too many open streams", f.Stream))
		return
	}

	ms := r.allocStream()
	c.addStream(f.Stream, m, ms)
	m.addStream(ms, c, f.Stream)

	if !m.send(tunnel.Frame{T: tunnel.TOpen, Stream: ms}) {
		// The member is wedged. Undo both sides and tell the client rather than
		// leaving it waiting for an accept that will never come.
		m.dropStream(ms)
		c.dropStream(f.Stream)
		c.send(errFrame(tunnel.ErrMachineOffline, "machine "+f.Machine+" is not responding", f.Stream))
		return
	}
	c.send(tunnel.Frame{T: tunnel.TAccept, Machine: f.Machine, Stream: f.Stream})
}

func (r *Relay) routeFromClient(c *clientConn, f tunnel.Frame) {
	owner, ok := c.stream(f.Stream)
	if !ok {
		c.send(errFrame(tunnel.ErrBadRequest, "unknown stream", f.Stream))
		return
	}
	owner.member.send(tunnel.Frame{T: tunnel.TData, Stream: owner.memberStream, Payload: f.Payload})
}

func (r *Relay) handleClientClose(c *clientConn, f tunnel.Frame) {
	owner, ok := c.dropStream(f.Stream)
	if !ok {
		return
	}
	owner.member.send(tunnel.Frame{T: tunnel.TClose, Stream: owner.memberStream})
	owner.member.dropStream(owner.memberStream)
}

// ---------------------------------------------------------------- presence

// Shutdown closes every member and client link. It is a routing outage, not
// data loss: the sessions live in the members, so each one redials and its
// output ring still holds whatever was produced meanwhile.
func (r *Relay) Shutdown(reason string) {
	r.mu.Lock()
	ms := make([]*member, 0, len(r.members))
	for _, m := range r.members {
		ms = append(ms, m)
	}
	r.mu.Unlock()

	r.rosterMu.Lock()
	cs := make([]*clientConn, 0, len(r.roster))
	for c := range r.roster {
		cs = append(cs, c)
	}
	r.rosterMu.Unlock()

	for _, m := range ms {
		m.close(reason)
	}
	for _, c := range cs {
		c.close(reason)
	}
}

// Machines serves the member list as JSON. It is the same snapshot clients
// receive over the tunnel, exposed for smoke tests and operator poking.
func (r *Relay) Machines(w http.ResponseWriter, rq *http.Request) {
	if !r.tokenOK(r.headerToken(rq)) {
		ip := clientIP(rq, r.cfg.TrustProxy)
		r.lim.fail(ip)
		r.log.Printf("relay: rejected roster request from %s", ip)
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"machines": r.snapshot()})
}

func (r *Relay) broadcastMachines() {
	snap := r.snapshot()
	r.rosterMu.Lock()
	cs := make([]*clientConn, 0, len(r.roster))
	for c := range r.roster {
		cs = append(cs, c)
	}
	r.rosterMu.Unlock()
	for _, c := range cs {
		c.send(tunnel.Frame{T: tunnel.TMachines, Machines: snap})
	}
}

// ---------------------------------------------------------------- plumbing

func errFrame(code, msg string, stream uint32) tunnel.Frame {
	return tunnel.Frame{T: tunnel.TError, Code: code, Message: msg, Stream: stream}
}

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

func writeFrame(ctx context.Context, conn *websocket.Conn, f tunnel.Frame) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(wctx, websocket.MessageText, b)
}

// writeLoop drains a peer's outbound queue. A peer that cannot keep up is
// closed rather than allowed to stall the relay for everyone else.
func (r *Relay) writeLoop(ctx context.Context, conn *websocket.Conn, out <-chan tunnel.Frame, done <-chan struct{}, onErr func()) {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case f := <-out:
			if err := writeFrame(ctx, conn, f); err != nil {
				onErr()
				return
			}
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, pingTimeout)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				onErr()
				return
			}
		}
	}
}

// ---------------------------------------------------------------- member

type streamRef struct {
	client       *clientConn
	clientStream uint32
}

type member struct {
	r    *Relay
	id   string
	name string
	host protocol.Host
	conn *websocket.Conn
	out  chan tunnel.Frame
	done chan struct{}
	once sync.Once

	mu      sync.Mutex
	streams map[uint32]*streamRef
}

func (m *member) send(f tunnel.Frame) bool {
	select {
	case <-m.done:
		return false
	case m.out <- f:
		return true
	default:
		m.close("outbound queue full")
		return false
	}
}

func (m *member) addStream(ms uint32, c *clientConn, cs uint32) {
	m.mu.Lock()
	m.streams[ms] = &streamRef{client: c, clientStream: cs}
	m.mu.Unlock()
}

func (m *member) dropStream(ms uint32) {
	m.mu.Lock()
	delete(m.streams, ms)
	m.mu.Unlock()
}

// forwardToClient copies one member frame to whichever client owns that
// stream. The relay only rewrites the addressing: the payload is the member's,
// byte for byte. It is the mirror of routeFromClient, and the two are kept
// apart deliberately — confusing them sends a client's keystrokes back to the
// client and looks, from the outside, like a session that never answers.
func (m *member) forwardToClient(f tunnel.Frame) {
	m.mu.Lock()
	ref, ok := m.streams[f.Stream]
	m.mu.Unlock()
	if !ok {
		return
	}
	ref.client.send(tunnel.Frame{
		T:       f.T,
		Machine: m.id,
		Stream:  ref.clientStream,
		Payload: f.Payload,
		Code:    f.Code,
		Message: f.Message,
	})
}

func (m *member) close(reason string) {
	m.once.Do(func() {
		close(m.done)
		_ = m.conn.Close(websocket.StatusNormalClosure, reason)

		// Tell every client watching this member that its streams are gone.
		m.mu.Lock()
		refs := make([]*streamRef, 0, len(m.streams))
		for _, ref := range m.streams {
			refs = append(refs, ref)
		}
		m.streams = map[uint32]*streamRef{}
		m.mu.Unlock()
		for _, ref := range refs {
			ref.client.send(tunnel.Frame{
				T:       tunnel.TClose,
				Machine: m.id,
				Stream:  ref.clientStream,
				Code:    tunnel.ErrMachineOffline,
				Message: "machine disconnected",
			})
			ref.client.dropStream(ref.clientStream)
		}
	})
}

// ---------------------------------------------------------------- client

type stream struct {
	member       *member
	memberStream uint32
}

type clientConn struct {
	r    *Relay
	conn *websocket.Conn
	out  chan tunnel.Frame
	done chan struct{}
	once sync.Once

	mu      sync.Mutex
	streams map[uint32]*stream
}

func (c *clientConn) send(f tunnel.Frame) {
	select {
	case <-c.done:
	case c.out <- f:
	default:
		c.close("outbound queue full")
	}
}

func (c *clientConn) addStream(cs uint32, m *member, ms uint32) {
	c.mu.Lock()
	c.streams[cs] = &stream{member: m, memberStream: ms}
	c.mu.Unlock()
}

func (c *clientConn) stream(cs uint32) (stream, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.streams[cs]
	if !ok {
		return stream{}, false
	}
	return *s, true
}

func (c *clientConn) dropStream(cs uint32) (stream, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.streams[cs]
	if !ok {
		return stream{}, false
	}
	delete(c.streams, cs)
	return *s, true
}

func (c *clientConn) streamCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.streams)
}

func (c *clientConn) close(reason string) {
	c.once.Do(func() {
		close(c.done)
		_ = c.conn.Close(websocket.StatusNormalClosure, reason)

		// Detach this client from every member it was watching. The members
		// keep their sessions: a viewer going away is a detach, never a close.
		c.mu.Lock()
		cs := c.streams
		c.streams = map[uint32]*stream{}
		c.mu.Unlock()
		for _, s := range cs {
			s.member.send(tunnel.Frame{T: tunnel.TClose, Stream: s.memberStream})
			s.member.dropStream(s.memberStream)
		}
	})
}
