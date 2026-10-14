// Command tfd is the headless half of term-fleet: it hosts PTY sessions on this
// machine and hands them to any client that connects, over one WebSocket.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/CarlOpenLab/term-fleet/internal/daemon"
	"github.com/CarlOpenLab/term-fleet/internal/hub"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7678",
		"address to listen on; loopback unless you mean it (off loopback requires -token)")
	token := flag.String("token", "",
		"shared secret every client must present; required off loopback")
	origins := flag.String("allow-origin", "",
		"comma-separated extra WebSocket Origin patterns, for a frontend served from another port")
	idle := flag.Duration("idle-controller", hub.DefaultIdleController,
		"how long a silent controller keeps control")
	webDir := flag.String("web-dir", "",
		"serve the browser client from this directory instead of the embedded copy")
	flag.Parse()

	srv, err := daemon.New(daemon.Options{
		Addr:           *addr,
		Token:          *token,
		OriginPatterns: daemon.SplitList(*origins),
		IdleTimeout:    *idle,
		WebDir:         *webDir,
	})
	if err != nil {
		log.Fatalf("tfd: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("tfd: listening on %s (%s/%s)", srv.Addr(), runtime.GOOS, runtime.GOARCH)
	if srv.ClientPresent() {
		log.Printf("tfd: client %s at %s/", srv.ClientSource(), srv.BaseURL())
	} else {
		log.Printf("tfd: no built client; run `pnpm --dir web run build`, or pass -web-dir")
	}
	if *token == "" {
		log.Printf("tfd: no token set; reachable from this machine only")
	}

	if err := srv.Serve(ctx); err != nil {
		log.Fatalf("tfd: %v", err)
	}
}