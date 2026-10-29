// Command tfrelay is the public-side router of a beacon fleet.
//
// Machines dial out to it; browsers and apps dial in. Nothing listens on the
// member side, which is what lets a fleet sit behind NAT.
//
//	tfrelay enroll -name "mac mini"      # mint a credential for one machine
//	tfrelay list                         # show enrolled machines
//	tfrelay -addr :7698 -tls-cert ... -tls-key ...   # run the relay
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CarlOpenLab/beacon/internal/accounts"
	"github.com/CarlOpenLab/beacon/internal/admin"
	"github.com/CarlOpenLab/beacon/internal/diag"
	"github.com/CarlOpenLab/beacon/internal/relay"
	"github.com/CarlOpenLab/beacon/internal/web"
)

func main() {
	log.SetFlags(log.Ltime)

	// Subcommands come first: enrolling a machine is an offline operation and
	// should not require, or touch, a listening socket.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "enroll":
			runEnroll(os.Args[2:])
			return
		case "list":
			runList(os.Args[2:])
			return
		case "help", "-h", "--help":
			usage()
			return
		}
	}
	runServe(os.Args[1:])
}

func usage() {
	fmt.Fprint(os.Stderr, `tfrelay — public-side router for a beacon fleet

  tfrelay enroll -name <machine> [-tenant <user-id>] [-devices devices.json]
  tfrelay list [-devices devices.json]
  tfrelay [-addr host:port] [-devices devices.json] -client-token <token>
          [-tls-cert cert.pem -tls-key key.pem] [-insecure]

Running the relay also serves a web admin panel at -admin-path (default
/admin), where machines are enrolled, the client token is rotated, registered
users are approved, and the CA certificate is downloaded. The enroll and list
subcommands remain for scripted setup.

Multi-tenant: with -users set, the relay serves an open signup and a login at
/api, and every registered user sees only the machines enrolled under their
tenant. Bootstrap the super admin with -admin-user/-admin-password (or
$TFRELAY_ADMIN_USER / $TFRELAY_ADMIN_PASSWORD) so approval is possible on a
fresh deployment without opening the setup wizard.
`)
}

func runEnroll(args []string) {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	devices := fs.String("devices", "devices.json", "path to the member registry")
	name := fs.String("name", "", "human name for this machine (required)")
	tenant := fs.String("tenant", "", "tenant id (a registered user's id) the machine belongs to; empty for the operator's own scope")
	_ = fs.Parse(args)
	if *name == "" {
		fmt.Fprintln(os.Stderr, "enroll: -name is required")
		os.Exit(2)
	}
	id, secret, err := relay.EnrollDevice(*devices, *name, *tenant, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		log.Fatalf("enroll: %v", err)
	}
	fmt.Printf("enrolled %s (%s)\n\n", id, *name)
	fmt.Printf("  put these on that machine's agent:\n")
	fmt.Printf("    -relay <wss://relay.example/relay/agent>\n")
	fmt.Printf("    -device-id %s\n", id)
	fmt.Printf("    -device-secret %s\n\n", secret)
	fmt.Printf("  the secret is shown once; the registry at %s holds it (mode 0600).\n", *devices)
}

func runList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	devices := fs.String("devices", "devices.json", "path to the member registry")
	_ = fs.Parse(args)
	ids, err := relay.ListDevices(*devices)
	if err != nil {
		log.Fatalf("list: %v", err)
	}
	if len(ids) == 0 {
		fmt.Println("no machines enrolled")
		return
	}
	for _, id := range ids {
		fmt.Println(id)
	}
}

func runServe(args []string) {
	addr := flag.String("addr", "127.0.0.1:7698", "listen address; loopback unless you mean it")
	devices := flag.String("devices", "devices.json", "path to the member registry")
	clientToken := flag.String("client-token", os.Getenv("TFRELAY_CLIENT_TOKEN"), "shared secret clients must present (required; defaults to $TFRELAY_CLIENT_TOKEN)")
	tokenFile := flag.String("token-file", "", "where a rotated client token is persisted (default: token.txt beside -devices)")
	tokenTTL := flag.Duration("client-token-ttl", relay.DefaultClientTokenTTL, "how long a client token minted from the panel stays valid; 0 means it never expires")
	controlTTL := flag.Duration("control-token-ttl", relay.DefaultControlTokenTTL, "how long a credential minted for an enrolled machine stays valid; it is what lets a desktop reach the other machines without the shared token")
	adminOn := flag.Bool("admin", true, "serve the web admin panel")
	adminPath := flag.String("admin-path", "/admin", "URL path the admin panel is mounted at")
	adminStore := flag.String("admin-store", "", "admin account file (default: admin.json beside -devices)")
	adminUser := flag.String("admin-user", os.Getenv("TFRELAY_ADMIN_USER"), "bootstrap the super admin with this username when no account exists yet (defaults to $TFRELAY_ADMIN_USER)")
	adminPassword := flag.String("admin-password", os.Getenv("TFRELAY_ADMIN_PASSWORD"), "bootstrap the super admin with this password (defaults to $TFRELAY_ADMIN_PASSWORD; never in a process list you do not trust)")
	usersFile := flag.String("users", "", "registered-user store enabling multi-tenant signup (default: users.json beside -devices; pass \"off\" to disable)")
	userTokenTTL := flag.Duration("user-token-ttl", accounts.DefaultTokenTTL, "how long a registered user's login token stays valid")
	publicURL := flag.String("public-url", "", "relay URL shown in the panel, e.g. wss://relay.example/relay/agent (default: guessed from the request)")
	caCert := flag.String("ca-cert", "", "PEM certificate offered for download from the panel, for members that must pin it")
	tlsCert := flag.String("tls-cert", "", "TLS certificate; enables HTTPS/WSS")
	tlsKey := flag.String("tls-key", "", "TLS private key")
	insecure := flag.Bool("insecure", false, "allow plain HTTP off loopback (development only)")
	origins := flag.String("origins", "", "comma-separated allowed Origin hosts; empty means same-host only")
	webDir := flag.String("web-dir", "", "serve the client from this directory instead of the embedded copy")
	trustProxy := flag.Bool("trust-proxy", false, "read X-Forwarded-For for rate limiting and logs (only behind a proxy you control)")
	maxFailures := flag.Int("auth-max-failures", 10, "rejected credentials per address before a lockout; 0 disables")
	logDir := flag.String("log-dir", "", "directory for relay logs and persisted machine log reports (default: logs beside -devices)")
	flag.Parse()

	stateDir := filepath.Dir(*devices)
	if *tokenFile == "" {
		*tokenFile = filepath.Join(stateDir, "token.txt")
	}
	if *adminStore == "" {
		*adminStore = filepath.Join(stateDir, "admin.json")
	}
	if *logDir == "" {
		*logDir = filepath.Join(stateDir, "logs")
	}
	if *usersFile == "" {
		*usersFile = filepath.Join(stateDir, "users.json")
	}
	if _, err := diag.Install(*logDir, "tfrelay", diag.Info); err != nil {
		log.Printf("tfrelay: diagnostics are not being written to disk: %v", err)
	}

	if *clientToken == "" && !fileExists(*tokenFile) {
		log.Fatal("tfrelay: -client-token is required: this relay fronts remote shells")
	}
	tlsOn := *tlsCert != "" && *tlsKey != ""
	if (*tlsCert == "") != (*tlsKey == "") {
		log.Fatal("tfrelay: -tls-cert and -tls-key must be given together")
	}
	if err := validateBinding(*addr, tlsOn, *insecure); err != nil {
		log.Fatalf("tfrelay: %v", err)
	}

	// The registry is opened, not snapshotted: machines are enrolled from the
	// admin panel while the relay runs, and a credential minted there has to be
	// accepted on the very next dial.
	reg, err := relay.OpenRegistry(*devices)
	if err != nil {
		log.Fatalf("tfrelay: %v", err)
	}
	token, err := relay.OpenTokenStore(*tokenFile, *clientToken)
	if err != nil {
		log.Fatalf("tfrelay: %v", err)
	}
	if token.Get() == "" {
		log.Fatal("tfrelay: no client token: pass -client-token or set $TFRELAY_CLIENT_TOKEN")
	}
	logs, err := relay.NewPersistentLogStore(filepath.Join(*logDir, "machines"))
	if err != nil {
		log.Fatalf("tfrelay: open machine log store: %v", err)
	}

	var patterns []string
	if *origins != "" {
		patterns = splitAndTrim(*origins)
	}
	r, err := relay.New(relay.Config{
		Registry:        reg,
		Token:           token,
		OriginPatterns:  patterns,
		TrustProxy:      *trustProxy,
		AuthMaxFailures: *maxFailures,
		ControlTokenTTL: *controlTTL,
		LogStore:        logs,
		Log:             log.Default(),
	})
	if err != nil {
		log.Fatalf("tfrelay: %v", err)
	}

	// Multi-tenant: a users.json turns signup on. Login tokens verify through
	// the token store, so a registered user's credential rides exactly the
	// same admission path as the shared token or a machine's — one funnel, one
	// rate limiter, one place an expired credential stops working.
	var users *accounts.Store
	if *usersFile != "off" {
		users, err = accounts.Open(*usersFile, *userTokenTTL)
		if err != nil {
			log.Fatalf("tfrelay: users: %v", err)
		}
		token.SetLoginVerifier(users)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", r.ServeAgent)
	mux.HandleFunc("/relay/client", r.ServeClient)
	mux.HandleFunc("/relay/machines", r.Machines)
	mux.HandleFunc("/relay/control-token", r.ControlToken)
	mux.HandleFunc("/relay/log", r.ServeLog)
	// The relay serves the same client bundle a daemon does, and tells it which
	// transport to use. Without this a phone could not reach a fleet at all: the
	// relay is the only address that is reachable from outside the LAN.
	mux.HandleFunc("/tf/config", web.Config("relay", "/relay/client"))

	if users != nil {
		accounts.New(users, r, log.Default()).Register(mux)
		log.Printf("tfrelay: signup and login at /api/register and /api/login (users in %s)", *usersFile)
	}

	if *adminOn {
		panel, err := admin.New(admin.Options{
			Path:       *adminStore,
			Relay:      r,
			Users:      users,
			PublicURL:  *publicURL,
			CACertPath: *caCert,
			BasePath:   *adminPath,
			TokenTTL:   *tokenTTL,
			Log:        log.Default(),
		})
		if err != nil {
			log.Fatalf("tfrelay: admin: %v", err)
		}
		// A deployment that ships credentials initializes its super admin at
		// startup, so approval is possible before anyone opens the panel. Only
		// ever a bootstrap: an existing account is never overwritten here.
		if !panel.Configured() && *adminUser != "" && *adminPassword != "" {
			if err := panel.Bootstrap(*adminUser, *adminPassword); err != nil {
				log.Fatalf("tfrelay: admin bootstrap: %v", err)
			}
			log.Printf("tfrelay: super admin %q bootstrapped from environment/flags", *adminUser)
		}
		panel.Register(mux)
		if panel.Configured() {
			log.Printf("tfrelay: admin panel at %s (account %q)", *adminPath, panel.Username())
		} else {
			log.Printf("tfrelay: admin panel at %s — open it to create the admin account, or set $TFRELAY_ADMIN_USER/$TFRELAY_ADMIN_PASSWORD", *adminPath)
		}
	}

	// Probed by systemd and by whatever terminates TLS in front of this. It
	// reports online members too, which is the one number worth watching.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintf(w, `{"ok":true,"online":%d}`, r.Online())
	})
	mux.Handle("/", web.Handler(*webDir))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	scheme := "http"
	if tlsOn {
		scheme = "https"
	}
	log.Printf("tfrelay: %d machine(s) enrolled; listening on %s://%s", reg.Len(), scheme, *addr)
	if reg.Len() == 0 {
		log.Printf("tfrelay: no machines yet — enrol them from the admin panel")
	}
	if !tlsOn {
		log.Printf("tfrelay: TLS is off — put this behind a TLS terminator before exposing it")
	}

	var serveErr error
	if tlsOn {
		serveErr = srv.ListenAndServeTLS(*tlsCert, *tlsKey)
	} else {
		serveErr = srv.ListenAndServe()
	}
	log.Fatalf("tfrelay: %v", serveErr)
}

// validateBinding refuses to expose an unauthenticated, unencrypted remote
// shell. It mirrors tfd's stance: a misconfiguration is an error, not a
// warning the operator can scroll past.
func validateBinding(addr string, tlsOn, insecure bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("bad -addr %q: %w", addr, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		if !tlsOn && !insecure {
			return fmt.Errorf("refusing to listen on %s without TLS: pass -tls-cert/-tls-key, or -insecure for a development box", addr)
		}
		return nil
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	if !tlsOn && !insecure {
		return fmt.Errorf("refusing to listen on %s without TLS: pass -tls-cert/-tls-key, or -insecure for a development box", addr)
	}
	return nil
}

func splitAndTrim(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Size() > 0
}
