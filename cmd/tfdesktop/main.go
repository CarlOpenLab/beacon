// Command tfdesktop is Beacon's PC client: the daemon `tfd` runs, with a
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
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/diag"
	"github.com/CarlOpenLab/beacon/internal/settings"
	"github.com/CarlOpenLab/beacon/locales"
)

func main() {
	// One instance, one window: starting the app again raises the window that
	// is already open instead of starting a second one — a second hub serving
	// the same machine's shells, a second relay link under the same identity.
	// A macOS bundle is single by the time Finder launches it; this is what
	// makes Windows and Linux agree with it, and what keeps a launch from a
	// terminal from doubling the app there too.
	//
	// A copy pointed at a settings home of its own — a portable copy, which
	// settings.DirOverridden reports — is a Beacon of its own, with its own
	// relay enrollment, so it is left to run beside the installed one.
	if !settings.DirOverridden() {
		if !mygo.App.RequestSingleInstanceLock() {
			return
		}
	}

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

	o := parseOptions(os.Args[1:], os.Stderr, flag.ExitOnError)

	// Ctrl-C in a terminal, or the signal a service manager sends, shuts the
	// window's shells down the same way closing the window does.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store := settings.Open(settings.Dir())
	saved := store.Get()

	// The bundle knows the app's version; a `go run` does not. Everything that
	// reports this build — the handshake, the relay's roster, the log's first
	// line — reads the same answer, so a machine's version on another screen is
	// the version in the file it will be asked to send.
	versionOf = func() string {
		if v := strings.TrimSpace(mygo.App.Version()); v != "" {
			return v
		}
		return diag.BuildVersion()
	}

	// The diagnostic log comes up before anything that could go wrong: a window
	// with no visible console is exactly the program that needs a file to say
	// what happened. Detailed is the default rather than something to turn on
	// first — a blank pane is not reproducible on demand, and asking someone to
	// reproduce it again with logging on is asking for a second bug report. The
	// hot paths summarise themselves (see internal/diag and termConn.feed), so
	// the file stays small; the flag is there to quieten it when it does not.
	level := diag.Debug
	if o.logLevel != "" {
		parsed, err := diag.ParseLevel(o.logLevel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tfdesktop: -log-level: %v\n", err)
			os.Exit(2)
		}
		level = parsed
	}
	logPath := o.logDir
	if logPath == "" {
		logPath = filepath.Join(store.Dir(), "logs")
	}
	logger, logErr := diag.Install(logPath, "tfdesktop", level)
	if logErr != nil {
		log.SetFlags(0)
		log.Printf("tfdesktop: diagnostics are off: %v", logErr)
	} else {
		defer func() { _ = logger.Close() }()
		diag.Infof("app", "start version=%s %s/%s go=%s log=%s level=%s",
			versionOf(), runtime.GOOS, runtime.GOARCH, runtime.Version(), logger.Path(), level)
	}

	// startupErr is why the local service did not come up, if it did not. The
	// window shows it once, because a service that silently failed leaves a
	// machine that looks joined but answers nothing.
	var startupErr error

	f := newFleet(ctx, log.Default())
	f.addr = o.addr
	f.webDir = o.webDir

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
	// The appearance, before the first frame: the window's material follows the
	// mode the person chose, or the system's when they chose nothing.
	applyAppearance(saved.Theme)
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

	app := newWindow(f, store, o.shell, o.cwd)
	// A command line that named a directory opens the window's first shell
	// there; one that named none leaves New Tab to choose, as it always did.
	wd, _ := os.Getwd()
	app.startupDir = resolveDir(o.dir, wd)
	// A second launch — the taskbar icon double-clicked, the exe run again from
	// a shell — was handed to this instance by RequestSingleInstanceLock. There
	// is one window, so the answer is to raise it rather than to open another;
	// a line that named a directory also opens a tab there, which is what
	// "beacon ." means when Beacon is already up.
	mygo.App.OnSecondInstance(app.handleSecondInstance)
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
		win := mygo.NewWindow(framedWindow(mygo.WindowOptions{
			Title:           "Beacon",
			Width:           1180,
			Height:          760,
			MinWidth:        720,
			MinHeight:       420,
			BackgroundColor: windowGround(saved.Theme, mygo.Theme.IsDark()),
			Vibrancy:        vibrancyFor(saved.Blur()),
			StateKey:        "main",
			Content:         ui.View(app.view),
		}))
		win.OnClosed(func() {
			stop()
			mygo.App.Quit()
		})
		// The maximize control's drawing changes with the state, and the state
		// can change from outside the view (a double click on the header, the
		// system's snap). Redraw when it does.
		win.OnMaximize(app.invalidate)
		win.OnUnmaximize(app.invalidate)
		app.win = win
		// After the window exists and the loop is up: the updater item asks the
		// plugin, and the menu's 设置… item reopens this window.
		installMenu(app)
		// Open with one shell, as a terminal app does. What the hub says about
		// it — and about every other shell on this machine — arrives on the
		// window's own link, so there is no polling loop to run. The tab is
		// window state, so it is opened on the window's thread.
		app.win.Update(app.startFirstTab)
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatalf("tfdesktop: %v", err)
	}
	stop()
}

// options is one command line: the flags the app was started with. The first
// instance parses its own; a later instance's line arrives through
// RequestSingleInstanceLock and is read by the same function, so "beacon ."
// means the same thing whether the app was running or not.
type options struct {
	addr     string
	webDir   string
	shell    string
	cwd      string
	logLevel string
	logDir   string
	// dir is the first positional argument: the directory to open a tab in.
	// Empty when the line named none.
	dir string
}

// parseOptions reads a command line. onError decides what a flag this build
// does not know does: the first launch exits with usage (flag.ExitOnError),
// while a line handed over by a newer build is read for the arguments this
// build understands instead of dropping the whole request.
func parseOptions(args []string, output io.Writer, onError flag.ErrorHandling) *options {
	fs := flag.NewFlagSet("tfdesktop", onError)
	fs.SetOutput(output)
	o := &options{}
	fs.StringVar(&o.addr, "addr", "127.0.0.1:0",
		"loopback address for the daemon under the window; port 0 picks a free one")
	fs.StringVar(&o.webDir, "web-dir", "",
		"serve the browser client from this directory instead of the embedded copy")
	fs.StringVar(&o.shell, "shell", "",
		"shell to run in new tabs (default: the login shell)")
	fs.StringVar(&o.cwd, "cwd", "",
		"working directory for new tabs (default: this machine's home directory)")
	fs.StringVar(&o.logLevel, "log-level", "",
		"diagnostic log level: error, warn, info or debug (default: info, or debug when Settings → Diagnostics asked for it)")
	fs.StringVar(&o.logDir, "log-dir", "",
		"where diagnostic logs are written (default: <settings dir>/logs)")
	_ = fs.Parse(args)
	if rest := fs.Args(); len(rest) > 0 {
		o.dir = rest[0]
	}
	return o
}

// forwardedDir is the directory a second launch asked for: its first
// positional argument, else -cwd, resolved against the shell that typed it.
// Empty when the line asked for nothing but the window.
func forwardedDir(args []string, wd string) string {
	o := parseOptions(args, io.Discard, flag.ContinueOnError)
	dir := o.dir
	if dir == "" {
		dir = o.cwd
	}
	return resolveDir(dir, wd)
}

// resolveDir turns a command line's directory into an absolute one. A relative
// path belongs to the shell that typed it, so it is joined to that shell's
// working directory — not this process's, which for an app the window server
// started is a place nobody chose. A file names its directory, so
// "beacon main.go" opens a shell beside the file. Empty stays empty.
func resolveDir(dir, wd string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	if !filepath.IsAbs(dir) {
		base := wd
		if base == "" {
			base, _ = os.Getwd()
		}
		if base != "" {
			dir = filepath.Join(base, dir)
		}
	}
	dir = filepath.Clean(dir)
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		return filepath.Dir(dir)
	}
	return dir
}
