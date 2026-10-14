package main

import (
	"context"
	"crypto/x509"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/agent"
	"github.com/CarlOpenLab/term-fleet/internal/daemon"
	"github.com/CarlOpenLab/term-fleet/internal/hub"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
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

	mu        sync.Mutex
	srv       *daemon.Server
	stopSrv   func()
	stopAgent func()
	relayOn   bool
}

// newFleet builds the hub and starts the goroutine that keeps it reaping idle
// sessions for the life of ctx.
func newFleet(ctx context.Context, log *log.Logger) *fleet {
	hostname, _ := os.Hostname()
	h := hub.New(hub.Options{
		Host: protocol.Host{
			Hostname: hostname,
			Platform: runtime.GOOS,
			Arch:     runtime.GOARCH,
		},
	})
	go h.Run(ctx)
	return &fleet{ctx: ctx, hub: h, log: log, addr: loopback}
}

// Hub is the session authority every client — window, browser, relay — shares.
func (f *fleet) Hub() *hub.Hub { return f.hub }

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
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(f.ctx)
	f.stopAgent = cancel
	f.relayOn = true
	f.log.Printf("fleet: joining %s as %q", relayURL, deviceID)
	go func() {
		if err := a.Run(ctx); err != nil && ctx.Err() == nil {
			f.log.Printf("agent: %v", err)
		}
	}()
	return nil
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
}
