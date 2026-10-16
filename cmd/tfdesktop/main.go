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

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/settings"
)

func main() {
	// Auto-update: the app checks its own release feed in the background and
	// offers new versions with their release notes (install / remind me later /
	// skip this version). The native plugin draws that window with mygo's own UI
	// rather than a web page, which is what the rest of this app does too — no
	// WebKitGTK, no WebView2 Runtime. Builds that were never signed for updates
	// (a development build, or a .deb a package manager installed) say so
	// instead of checking.
	mygo.Use(native.Plugin)

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

	store := settings.Open(settings.Dir())
	saved := store.Get()

	f := newFleet(ctx, log.Default())
	f.addr = *addr
	f.webDir = *webDir

	// Apply what was saved before the window opens, so the first tab already
	// has the right font and the machine is already on the relay it joined.
	if err := f.SetServer(!saved.ServerOff); err != nil {
		log.Printf("tfdesktop: local service: %v", err)
	}
	// Who may drive this machine's shells, applied before anything can reach it:
	// a machine that was left read-only for other clients must not be reachable
	// for the moment it takes the window to open.
	f.SetRemoteControl(!saved.ControlOff)
	if err := f.SetRelay(saved.RelayURL, saved.DeviceID, saved.DeviceSecret, saved.RelayCA, saved.RelayOn); err != nil {
		log.Printf("tfdesktop: relay: %v", err)
	}
	// The window's own client link to the relay: the roster of other machines,
	// and the streams that reach their shells. It is separate from the member
	// link above and needs the fleet's client token, not this machine's
	// credential.
	if err := f.SetRelayClient(saved.RelayURL, saved.ClientToken, saved.RelayCA); err != nil {
		log.Printf("tfdesktop: relay client: %v", err)
	}

	if !f.ClientPresent() {
		log.Printf("tfdesktop: no built client; run `pnpm --dir web run build` to serve one")
	}
	if url := f.BaseURL(); url != "" {
		log.Printf("tfdesktop: native window (%s/%s), browser client at %s/", runtime.GOOS, runtime.GOARCH, url)
	} else {
		log.Printf("tfdesktop: native window (%s/%s), local service off", runtime.GOOS, runtime.GOARCH)
	}

	// The terminal engine (libghostty-vt) loads on first use. Warm it here, off
	// the UI thread, so a first run that has to fetch it does not stall the
	// window right as the first tab appears.
	go func() {
		if err := terminal.Load(); err != nil {
			log.Printf("tfdesktop: terminal engine unavailable: %v", err)
		}
	}()

	app := newWindow(f, store, *shell, *cwd)
	// The link reports its own health from the agent's goroutine, so the pane
	// that shows it has to be told to repaint.
	f.onState = app.invalidateSettings

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "term-fleet",
			Width:           1180,
			Height:          760,
			MinWidth:        720,
			MinHeight:       420,
			BackgroundColor: windowGround(saved.Theme),
			StateKey:        "main",
			Content:         ui.View(app.view),
		})
		win.OnClosed(func() {
			stop()
			mygo.App.Quit()
		})
		app.win = win
		// After the window exists and the loop is up: the updater item asks the
		// plugin, and the menu's 设置… item reopens this window.
		installMenu(app)
		// Open with one shell, as a terminal app does. What the hub says about
		// it — and about every other shell on this machine — arrives on the
		// window's own link, so there is no polling loop to run. The tab is
		// window state, so it is opened on the window's thread.
		app.win.Update(app.newTab)
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatalf("tfdesktop: %v", err)
	}
	stop()
}
