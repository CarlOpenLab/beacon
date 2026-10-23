// Command tfd is the headless half of term-fleet: it hosts PTY sessions on this
// machine and hands them to any client that connects, over one WebSocket.
package main

import (
	"context"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/CarlOpenLab/term-fleet/internal/agent"
	"github.com/CarlOpenLab/term-fleet/internal/daemon"
	"github.com/CarlOpenLab/term-fleet/internal/diag"
	"github.com/CarlOpenLab/term-fleet/internal/hub"
	"github.com/CarlOpenLab/term-fleet/internal/settings"
)

func main() {
	// The service subcommand is not this daemon's flags but a way to install
	// it as one: it runs before flag.Parse, because the flags after it belong to
	// the service it writes, not to this invocation.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "service":
			serviceCommand(os.Args[2:])
			return
		case "version", "--version", "-version":
			fmt.Println("tfd " + diag.BuildVersion())
			return
		}
	}

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
	logLevel := flag.String("log-level", "info",
		"diagnostic log level: error, warn, info or debug; debug records every attach and replay")
	version := flag.Bool("version", false, "print the version and exit")
	logDir := flag.String("log-dir", "",
		"where diagnostic logs are written (default: <settings dir>/logs)")
	flag.Parse()
	if *version {
		fmt.Println("tfd " + diag.BuildVersion())
		return
	}

	// This machine's daemon is the one a blank pane is usually blamed on, and
	// it is often started by init/launchd/Windows service with no console at
	// all. The file is therefore the only place its side of the story can be
	// read, so it comes up before the hub exists.
	level, err := diag.ParseLevel(*logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tfd: -log-level: %v\n", err)
		os.Exit(2)
	}
	dir := *logDir
	if dir == "" {
		dir = filepath.Join(settings.Dir(), "logs")
	}
	logger, logErr := diag.Install(dir, "tfd", level)
	if logErr != nil {
		log.SetFlags(0)
		log.Printf("tfd: diagnostics are off: %v", logErr)
	} else {
		defer func() { _ = logger.Close() }()
		diag.Infof("app", "start version=%s %s/%s go=%s log=%s level=%s",
			diag.BuildVersion(), runtime.GOOS, runtime.GOARCH, runtime.Version(), logger.Path(), level)
	}

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
			Diag:        diag.Default(),
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
