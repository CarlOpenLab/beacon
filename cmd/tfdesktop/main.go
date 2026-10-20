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
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/settings"
	"github.com/CarlOpenLab/term-fleet/locales"
)

func main() {
	// Auto-update: the app checks its own release feed and offers new versions
	// with their release notes (install / remind me later / skip this version).
	// The native plugin draws that window with mygo's own UI rather than a web
	// page, which is what the rest of this app does too — no WebKitGTK, no
	// WebView2 Runtime. Builds that were never signed for updates (a development
	// build, or a .deb a package manager installed) say so instead of checking.
	//
	// The background check starts off: a terminal app that calls out to GitHub
	// every time it opens is a surprise, both as a firewall prompt and as a
	// privacy question. 设置 → 更新 turns it on, and 检查更新… in the menu works
	// either way.
	mygo.Use(native.New(updater.Options{DisableAutomaticChecks: true}))
	mygo.Theme.SetSource(mygo.ThemeDark)

	addr := flag.String("addr", "127.0.0.1:0",
		"loopback address for the daemon under the window; port 0 picks a free one")
	webDir := flag.String("web-dir", "",
		"serve the browser client from this directory instead of the embedded copy")
	shell := flag.String("shell", "",
		"shell to run in new tabs (default: the login shell)")
	cwd := flag.String("cwd", "",
		"working directory for new tabs (default: this machine's home directory)")
	flag.Parse()

	// Ctrl-C in a terminal, or the signal a service manager sends, shuts the
	// window's shells down the same way closing the window does.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store := settings.Open(settings.Dir())
	saved := store.Get()

	// startupErr is why the local service did not come up, if it did not. The
	// window shows it once, because a service that silently failed leaves a
	// machine that looks joined but answers nothing.
	var startupErr error

	f := newFleet(ctx, log.Default())
	f.addr = *addr
	f.webDir = *webDir

	// Apply what was saved before the window opens, so the first tab already
	// has the right font and the machine is already on the relay it joined.
	//
	// A failure here is kept for the window to show, not just logged: on macOS
	// the usual cause is the application firewall, and a person who finds the
	// service silently off has no way to connect the two. The log stays, because
	// it names the errno; the window states it in words.
	if err := f.SetServer(!saved.ServerOff); err != nil {
		log.Printf("tfdesktop: local service: %v", err)
		startupErr = err
	}
	// Who may drive this machine's shells, applied before anything can reach it:
	// a machine that was left read-only for other clients must not be reachable
	// for the moment it takes the window to open.
	f.SetRemoteControl(!saved.ControlOff)
	// The interface language, applied before the first frame is drawn: the
	// window's chrome says its first word in the language the person chose, or
	// in the one their system says they read.
	applyLanguage(saved.Language)
	if err := f.SetRelay(saved.RelayURL, saved.DeviceID, saved.DeviceSecret, saved.RelayCA, saved.RelayOn); err != nil {
		log.Printf("tfdesktop: relay: %v", err)
	}
	// The window's own client link to the relay: the roster of other machines,
	// and the streams that reach their shells. It is a second connection with a
	// credential of its own, asked for on the spot with the member credential
	// above, so reaching the fleet needs nothing typed into a settings screen.
	if saved.RelayOn && saved.RelayURL != "" {
		if err := f.Reach(); err != nil {
			log.Printf("tfdesktop: control token: %v", err)
		}
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
	// If the service could not start — the usual cause on macOS is the
	// application firewall — the window says so where it already says what the
	// fleet is doing, rather than leaving a machine that looks joined and
	// answers nothing.
	if startupErr != nil {
		app.setNotice(locales.AppServiceStartFailed(startupErr.Error()))
	}
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
			TitleBarStyle:   mygo.TitleBarHidden,
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
