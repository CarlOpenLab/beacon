// Package daemon assembles the hub and the browser client into one HTTP server.
//
// Both entry points use it: `tfd` runs it headless on a host, and `tfdesktop`
// runs it on loopback behind a native window. Keeping the wiring here means the
// desktop shell and a phone on the LAN are served by identical code.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/hub"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/web"
)

// Options configures a Server. The zero value listens on 127.0.0.1:0.
type Options struct {
	// Addr is the TCP address to bind. Port 0 lets the OS choose one, which is
	// what the desktop shell wants so it never collides with a running tfd.
	Addr string
	// Token is the shared secret every client must present. Required off loopback.
	Token string
	// OriginPatterns are extra accepted WebSocket Origin hosts, for a frontend
	// served from a different port during development.
	OriginPatterns []string
	// IdleTimeout is how long a silent controller keeps control.
	IdleTimeout time.Duration
	// WebDir serves the client from disk instead of the embedded copy.
	WebDir string
	// Client overrides the browser client handler. Tests and alternate frontends.
	Client http.Handler
}

// Server is a bound, not yet necessarily serving, term-fleet host.
type Server struct {
	hub *hub.Hub
	ln  net.Listener
	srv *http.Server

	clientPresent bool
	clientSource  string
}

// New validates the binding and binds the listener, so Addr and BaseURL are
// usable before Serve starts.
func New(opts Options) (*Server, error) {
	if err := ValidateBinding(opts.Addr, opts.Token); err != nil {
		return nil, err
	}
	hostname, _ := os.Hostname()
	h := hub.New(hub.Options{
		Host: protocol.Host{
			Hostname: hostname,
			Platform: runtime.GOOS,
			Arch:     runtime.GOARCH,
		},
		Token:          opts.Token,
		OriginPatterns: opts.OriginPatterns,
		IdleTimeout:    opts.IdleTimeout,
	})

	ln, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", opts.Addr, err)
	}

	client := opts.Client
	source := "来自 -web-dir 的磁盘产物"
	clientPresent := true
	switch {
	case client != nil:
		source = "外部注入"
	case opts.WebDir != "":
		client = web.Handler(opts.WebDir)
		source = opts.WebDir
	default:
		client = web.Handler("")
		if _, ok := web.FS(); ok {
			source = "内嵌"
		} else {
			clientPresent = false
			source = "缺失"
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.Handle("/ws", h)
	mux.HandleFunc("/tf/config", web.Config("direct", "/ws"))
	// The client shares the socket's origin, so the desktop shell and a phone
	// load identical code with no CORS arrangement.
	mux.Handle("/", client)

	return &Server{
		hub:           h,
		ln:            ln,
		srv:           &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second},
		clientPresent: clientPresent,
		clientSource:  source,
	}, nil
}

// Addr is the bound address, known before Serve runs.
func (s *Server) Addr() net.Addr { return s.ln.Addr() }

// Hub is the session hub, for callers that need to drive it directly.
func (s *Server) Hub() *hub.Hub { return s.hub }

// BaseURL is an address a WebView or browser on this machine can load. An
// unspecified or loopback bind is reported as 127.0.0.1.
func (s *Server) BaseURL() string {
	host, port, err := net.SplitHostPort(s.ln.Addr().String())
	if err != nil {
		return "http://" + s.ln.Addr().String()
	}
	switch {
	case host == "", host == "localhost":
		host = "127.0.0.1"
	default:
		if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
			host = "127.0.0.1"
		}
	}
	return "http://" + net.JoinHostPort(host, port)
}

// ClientPresent reports whether a built frontend is available to serve.
func (s *Server) ClientPresent() bool { return s.clientPresent }

// ClientSource describes where the frontend came from, for logs.
func (s *Server) ClientSource() string { return s.clientSource }

// Serve runs until ctx is cancelled or the listener fails. Cancelling ctx shuts
// the HTTP server down and terminates every hosted PTY.
func (s *Server) Serve(ctx context.Context) error {
	go s.hub.Run(ctx)
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(sctx)
	}()

	err := s.srv.Serve(s.ln)
	s.hub.Manager().Shutdown()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Close releases the listener without waiting for Serve.
func (s *Server) Close() error { return s.ln.Close() }

// ValidateBinding enforces the one rule that keeps this daemon from becoming an
// open remote shell: off loopback, a token is mandatory, not merely advisable.
func ValidateBinding(addr, token string) error {
	if IsLoopback(addr) {
		return nil
	}
	if token == "" {
		return fmt.Errorf("refusing to listen on %s without -token: this daemon runs arbitrary commands", addr)
	}
	return nil
}

// IsLoopback reports whether addr names the local machine.
func IsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// SplitList turns a comma-separated flag value into a slice, dropping blanks.
func SplitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
