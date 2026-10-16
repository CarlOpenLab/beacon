package main

import (
	"context"
	"crypto/x509"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/agent"
	"github.com/CarlOpenLab/term-fleet/internal/daemon"
	"github.com/CarlOpenLab/term-fleet/internal/hub"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/relayclient"
	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

// loopback is where the local service binds. Port 0 lets the OS choose, so the
// shell never collides with a tfd the person already has running.
const loopback = "127.0.0.1:0"

// fleet is what the window drives: one hub, and the two ways other clients
// reach it — the local HTTP service a browser on this machine uses, and the
// relay link a phone or another computer uses.
//
// The hub is created once and outlives both. Turning the local service off
// never costs the window its shells, and a relay that drops and reconnects
// keeps them too, because the hub is the session authority and nothing here
// terminates a session.
type fleet struct {
	ctx    context.Context
	hub    *hub.Hub
	log    *log.Logger
	addr   string
	webDir string

	// host is what this machine tells every client about itself in the
	// handshake. The window shows it in the machine panel, which is the one
	// place a person looks to see which machine they are on.
	host protocol.Host

	mu        sync.Mutex
	srv       *daemon.Server
	stopSrv   func()
	stopAgent func()
	relayOn   bool

	// client is the window's own link to the relay as a *client*: it carries the
	// roster of other machines and one stream per machine the window drives. It
	// is a second, separate connection from the member link above — the member
	// one is how this machine joins the fleet, this one is how the window
	// reaches the rest of it.
	client    *relayclient.Client
	clientErr string

	// onRoster, when set, is called with the relay's machine list on every
	// change. One subscription for the life of the fleet, so a link remade from
	// the settings window keeps feeding the same panel.
	onRoster func([]tunnel.Machine)

	// relayURL and relayDevice are the link as it was last configured: what the
	// agent is actually dialling and the device id it dials as. The panel shows
	// these rather than the store's copy, so what is on screen is what the link
	// is doing even when the settings window has a half-typed address in it.
	relayURL    string
	relayDevice string

	// relayState is the last thing the agent said about the link, kept so the
	// settings window can answer "why is this machine offline?" instead of
	// leaving a switch turned on with nothing else to go on. onState, when set,
	// asks the UI to repaint after it changes.
	relayState agent.State
	onState    func()
}

// newFleet builds the hub and starts the goroutine that keeps it reaping idle
// sessions for the life of ctx.
func newFleet(ctx context.Context, log *log.Logger) *fleet {
	hostname, _ := os.Hostname()
	host := protocol.Host{
		Hostname: hostname,
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
	}
	h := hub.New(hub.Options{Host: host})
	go h.Run(ctx)
	return &fleet{ctx: ctx, hub: h, log: log, addr: loopback, host: host}
}

// Hub is the session authority every client — window, browser, relay — shares.
func (f *fleet) Hub() *hub.Hub { return f.hub }

// Host is what this machine reports about itself.
func (f *fleet) Host() protocol.Host { return f.host }

// BaseURL is the local service's address, or "" while it is off.
func (f *fleet) BaseURL() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.srv == nil {
		return ""
	}
	return f.srv.BaseURL()
}

// ClientPresent reports whether a browser is currently attached to the local
// service.
func (f *fleet) ClientPresent() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.srv != nil && f.srv.ClientPresent()
}

// ServerOn reports whether the local service is listening.
func (f *fleet) ServerOn() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.srv != nil
}

// SetServer turns the local service on or off. Turning it off detaches local
// browser clients but leaves the shells running, so the window keeps what it is
// showing and turning the service back on re-lists the same sessions.
func (f *fleet) SetServer(on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case on && f.srv == nil:
		srv, err := daemon.New(daemon.Options{
			Addr:        f.addr,
			IdleTimeout: 30 * time.Minute,
			WebDir:      f.webDir,
			Hub:         f.hub,
		})
		if err != nil {
			return err
		}
		f.stopSrv = srv.Start()
		f.srv = srv
		f.log.Printf("fleet: local service at %s", srv.BaseURL())
	case !on && f.srv != nil:
		f.stopSrv()
		f.srv, f.stopSrv = nil, nil
		f.log.Printf("fleet: local service off")
	}
	return nil
}

// RelayOn reports whether a link to a relay is wanted. It stays true across a
// reconnect, since the agent retries on its own.
func (f *fleet) RelayOn() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.relayOn
}

// SetRemoteControl turns other clients' control of this machine's shells on or
// off. It is the machine's own hub that decides, so the switch reaches every
// client that reaches this machine — the LAN browser, a phone through a relay,
// another desktop — and takes the lease back from one of them mid-session. The
// window is never affected: it is the machine's own client.
func (f *fleet) SetRemoteControl(on bool) {
	f.hub.SetControlAllowed(on)
}

// SetRelay replaces the relay link. An empty URL, or enabled false, drops the
// current one; a bad credential is reported rather than retried in a loop, so
// the settings window can say what is wrong. caFile, when set, is the PEM the
// relay's TLS certificate must chain to — a private CA or a self-signed
// certificate, which a member cannot verify against the system roots.
func (f *fleet) SetRelay(relayURL, deviceID, secret, caFile string, enabled bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopAgent != nil {
		f.stopAgent()
		f.stopAgent, f.relayOn = nil, false
	}
	f.relayState = agent.State{}
	f.relayURL, f.relayDevice = "", ""
	if !enabled || relayURL == "" {
		return nil
	}
	var roots *x509.CertPool
	if caFile != "" {
		var err error
		roots, err = agent.LoadCACert(caFile)
		if err != nil {
			return err
		}
	}
	a, err := agent.New(agent.Config{
		URL:      relayURL,
		DeviceID: deviceID,
		Secret:   secret,
		RootCAs:  roots,
		Hub:      f.hub,
		Log:      f.log,
		OnState:  f.noteRelayState,
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(f.ctx)
	f.stopAgent = cancel
	f.relayOn = true
	f.relayURL, f.relayDevice = relayURL, deviceID
	f.log.Printf("fleet: joining %s as %q", relayURL, deviceID)
	go func() {
		if err := a.Run(ctx); err != nil && ctx.Err() == nil {
			f.log.Printf("agent: %v", err)
		}
	}()
	return nil
}

// SetRelayClient starts or replaces the window's client link to the relay. It
// needs the fleet's client token, which is not this machine's device
// credential: the member link proves this machine is itself, and the client
// link is what lets the window drive another machine's shells. An empty token
// drops the link.
func (f *fleet) SetRelayClient(relayURL, token, caFile string) error {
	f.mu.Lock()
	if f.client != nil {
		f.client.Close()
		f.client = nil
	}
	f.clientErr = ""
	f.mu.Unlock()

	if token == "" || relayURL == "" {
		return nil
	}
	// The callbacks go in with the connection: the roster can arrive while Dial
	// is still waiting for the welcome, and one assigned afterwards would miss it.
	f.mu.Lock()
	onRoster := f.onRoster
	f.mu.Unlock()

	c, err := relayclient.Dial(f.ctx, clientURL(relayURL), token,
		relayclient.Options{CAFile: caFile},
		relayclient.Hooks{
			OnRoster: onRoster,
			// The client link's own state is a reason to repaint: the machine
			// panel shows whether the fleet is reachable at all.
			OnState: func(relayclient.State, string) { f.noteClientState() },
		})
	f.mu.Lock()
	f.client = c
	if err != nil {
		f.clientErr = err.Error()
	}
	f.mu.Unlock()
	f.noteClientState()
	return err
}

// noteClientState asks whoever is watching for a repaint after the client link
// changes state.
func (f *fleet) noteClientState() {
	f.mu.Lock()
	notify := f.onState
	f.mu.Unlock()
	if notify != nil {
		notify()
	}
}

// RelayClient is the window's client link, or nil without a token.
func (f *fleet) RelayClient() *relayclient.Client {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.client
}

// RelayClientErr is why the client link is not up, or "".
func (f *fleet) RelayClientErr() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.clientErr
}

// clientURL is the client endpoint of a relay, from its member endpoint: the
// two live beside each other on the same host.
func clientURL(memberURL string) string {
	if i := strings.LastIndex(memberURL, "/"); i >= 0 {
		return memberURL[:i+1] + "client"
	}
	return memberURL
}

// RelayState reports what the agent last said about the link. It is what turns
// "the machine is still offline" into something actionable.
func (f *fleet) RelayState() agent.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.relayState
}

// RelayIdentity is the relay the link was given and the device id it dials as,
// empty while there is no link. It is what the machine panel shows.
func (f *fleet) RelayIdentity() (url, device string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.relayURL, f.relayDevice
}

// noteRelayState records a state change and asks for a repaint. The notify is
// called without the lock held: it ends up in UI code that reads RelayState
// back.
func (f *fleet) noteRelayState(s agent.State) {
	f.mu.Lock()
	f.relayState = s
	notify := f.onState
	f.mu.Unlock()
	if notify != nil {
		notify()
	}
}

// Close stops both links. It leaves the hub's sessions alone: the process is on
// its way out, and the OS reclaims the PTYs.
func (f *fleet) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopAgent != nil {
		f.stopAgent()
		f.stopAgent, f.relayOn = nil, false
	}
	if f.stopSrv != nil {
		f.stopSrv()
		f.srv, f.stopSrv = nil, nil
	}
	if f.client != nil {
		f.client.Close()
		f.client = nil
	}
}
