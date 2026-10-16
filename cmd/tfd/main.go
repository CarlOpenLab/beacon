// Command tfd is the headless half of term-fleet: it hosts PTY sessions on this
// machine and hands them to any client that connects, over one WebSocket.
package main

import (
	"context"
	"crypto/x509"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/CarlOpenLab/term-fleet/internal/agent"
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
	relayURL := flag.String("relay", "",
		"dial this relay member endpoint (e.g. wss://relay.example/relay/agent); empty keeps this machine LAN-only")
	deviceID := flag.String("device-id", "", "this machine's relay identity, from `tfrelay enroll`")
	deviceSecret := flag.String("device-secret", "", "this machine's relay secret, from `tfrelay enroll`")
	machineName := flag.String("machine-name", "", "name shown in the relay's machine list (default: hostname)")
	relayCA := flag.String("relay-ca", "",
		"PEM file with the certificate (or CA) that signed the relay's TLS certificate; use it when the relay is not behind a public CA")
	allowControl := flag.Bool("allow-control", true,
		"let other clients (a browser, a phone, another machine) drive the shells here; -allow-control=false leaves them read-only")
	flag.Parse()

	srv, err := daemon.New(daemon.Options{
		Addr:           *addr,
		Token:          *token,
		OriginPatterns: daemon.SplitList(*origins),
		IdleTimeout:    *idle,
		WebDir:         *webDir,
		ControlOff:     !*allowControl,
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

	// Joining a fleet is additive: the daemon keeps serving its LAN port either
	// way, so a machine that is reachable locally stays reachable locally even
	// when the relay is down.
	if *relayURL != "" {
		var roots *x509.CertPool
		if *relayCA != "" {
			roots, err = agent.LoadCACert(*relayCA)
			if err != nil {
				log.Fatalf("tfd: -relay-ca: %v", err)
			}
		}
		ag, err := agent.New(agent.Config{
			URL:         *relayURL,
			DeviceID:    *deviceID,
			Secret:      *deviceSecret,
			MachineName: *machineName,
			RootCAs:     roots,
			Hub:         srv.Hub(),
		})
		if err != nil {
			log.Fatalf("tfd: %v", err)
		}
		log.Printf("tfd: joining fleet at %s as %q", *relayURL, *deviceID)
		go func() {
			if err := ag.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("tfd: relay link stopped: %v", err)
			}
		}()
	}

	if err := srv.Serve(ctx); err != nil {
		log.Fatalf("tfd: %v", err)
	}
}
