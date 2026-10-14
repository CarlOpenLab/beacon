// Command tfdesktop is term-fleet's PC client: the daemon `tfd` runs, with a
// native window over it instead of a browser.
//
// The window is drawn with mygo's own UI toolkit and terminal — no webview, no
// xterm.js — so every tab is a real shell on this machine, rendered natively.
// The daemon still serves those same sessions over loopback, which is what lets
// a browser, or a phone through the relay, drive the very shells on screen.
//
// One consequence worth knowing: the PTYs are children of this process, so
// quitting the app ends them, as closing a terminal emulator does.
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/daemon"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:0",
		"loopback address for the daemon under the window; port 0 picks a free one")
	webDir := flag.String("web-dir", "",
		"serve the browser client from this directory instead of the embedded copy")
	shell := flag.String("shell", "",
		"shell to run in new tabs (default: the login shell)")
	cwd := flag.String("cwd", "",
		"working directory for new tabs (default: the process working directory)")
	flag.Parse()

	// Ctrl-C in a terminal, or the signal a service manager sends, shuts the
	// window's shells down the same way closing the window does.
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

	url := srv.BaseURL()
	if !srv.ClientPresent() {
		log.Printf("tfdesktop: no built client; run `pnpm --dir web run build` to serve one")
	}
	log.Printf("tfdesktop: native window (%s/%s), browser client at %s/", runtime.GOOS, runtime.GOARCH, url)

	// The terminal engine (libghostty-vt) loads on first use. Warm it here, off
	// the UI thread, so a first run that has to fetch it does not stall the
	// window right as the first tab appears.
	go func() {
		if err := terminal.Load(); err != nil {
			log.Printf("tfdesktop: terminal engine unavailable: %v", err)
		}
	}()

	app := newWindow(srv, *shell, *cwd)

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "term-fleet",
			Width:           1180,
			Height:          760,
			MinWidth:        720,
			MinHeight:       420,
			BackgroundColor: "#0b0e14",
			StateKey:        "main",
			Content:         ui.View(app.view),
		})
		win.OnClosed(func() {
			stop()
			mygo.App.Quit()
		})
		go app.watch(win)
		// Open with one shell, as a terminal app does.
		go app.create()
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatalf("tfdesktop: %v", err)
	}
	stop()
}
