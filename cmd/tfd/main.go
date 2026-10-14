// Command tfd is the term-fleet daemon: it owns PTY sessions on one machine
// and serves them over a single WebSocket to every client that attaches.
//
// It listens on loopback by default. The daemon runs arbitrary commands and
// serves the filesystem, so binding it to a network without a token is
// refused rather than warned about.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/hub"
	"github.com/CarlOpenLab/term-fleet/internal/protocol"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7678",
		"address to listen on; loopback unless you mean it")
	token := flag.String("token", "",
		"shared secret every client must present; required when not on loopback")
	origins := flag.String("allow-origin", "",
		"comma-separated extra WebSocket Origin patterns, for a dev server on another port")
	idle := flag.Duration("idle-controller", hub.DefaultIdleController,
		"how long a silent controller keeps control")
	flag.Parse()

	if err := validateBinding(*addr, *token); err != nil {
		log.Fatalf("tfd: %v", err)
	}

	hostname, _ := os.Hostname()
	h := hub.New(hub.Options{
		Host: protocol.Host{
			Hostname: hostname,
			Platform: runtime.GOOS,
			Arch:     runtime.GOARCH,
		},
		Token:          *token,
		OriginPatterns: splitList(*origins),
		IdleTimeout:    *idle,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.Handle("/ws", h)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("tfd: listen %s: %v", *addr, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go h.Run(ctx)

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()

	log.Printf("tfd: listening on %s (%s/%s)", ln.Addr(), runtime.GOOS, runtime.GOARCH)
	if *token == "" {
		log.Printf("tfd: no token set; reachable from this machine only")
	}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("tfd: serve: %v", err)
	}
	h.Manager().Shutdown()
}

// validateBinding enforces the one rule that keeps this daemon from becoming
// an open remote shell.
func validateBinding(addr, token string) error {
	if isLoopback(addr) {
		return nil
	}
	if token == "" {
		return fmt.Errorf("refusing to listen on %s without -token: this daemon runs arbitrary commands", addr)
	}
	return nil
}

func isLoopback(addr string) bool {
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

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
