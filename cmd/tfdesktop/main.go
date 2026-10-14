// Command tfdesktop is the PC client: the same daemon `tfd` runs, bound to
// loopback, behind a native window.
//
// The window loads the daemon's own origin instead of an embedded copy of the
// frontend. That keeps the desktop client, a browser tab, and (later) a phone
// on the LAN on byte-identical code, and lets this process host its own
// sessions rather than being a thin viewer of someone else's.
//
// One consequence worth knowing: the PTYs are children of this process, so
// quitting the app ends them, exactly like closing a terminal emulator.
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/daemon"
	"github.com/egoist/mygo"
)

func main() {
	// Port 0 so a desktop window never collides with a `tfd` already running on
	// this machine. Reach the same sessions from elsewhere with -web-dir? No:
	// use the `tfd` binary for a shared host.
	addr := flag.String("addr", "127.0.0.1:0",
		"loopback address for the embedded daemon; port 0 picks a free one")
	webDir := flag.String("web-dir", "",
		"serve the client from this directory instead of the embedded copy (dev loop)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv, err := daemon.New(daemon.Options{
		Addr:        *addr,
		IdleTimeout: 30 * time.Minute,
		WebDir:      *webDir,
	})
	if err != nil {
		log.Fatalf("tfdesktop: %v", err)
	}

	go func() {
		if err := srv.Serve(ctx); err != nil {
			log.Printf("tfdesktop: server stopped: %v", err)
		}
	}()

	url := srv.BaseURL() + "/"
	if !srv.ClientPresent() {
		log.Printf("tfdesktop: no built client; run `pnpm --dir web run build`")
	}
	log.Printf("tfdesktop: %s (%s/%s)", url, runtime.GOOS, runtime.GOARCH)

	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:           "term-fleet",
			URL:             url,
			Width:           1180,
			Height:          760,
			MinWidth:        720,
			MinHeight:       420,
			BackgroundColor: "#0b0e14",
			StateKey:        "main",
		})
	})

	if err := mygo.App.Run(); err != nil {
		stop()
		log.Fatalf("tfdesktop: %v", err)
	}

	// The window closed: stop the daemon, which ends every hosted session.
	stop()
}
