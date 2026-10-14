// Command tfrelay is the public-side router of a term-fleet.
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
	"strings"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/relay"
	"github.com/CarlOpenLab/term-fleet/internal/web"
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
	fmt.Fprint(os.Stderr, `tfrelay — public-side router for a term-fleet

  tfrelay enroll -name <machine> [-devices devices.json]
  tfrelay list [-devices devices.json]
  tfrelay [-addr host:port] [-devices devices.json] -client-token <token>
          [-tls-cert cert.pem -tls-key key.pem] [-insecure]
`)
}

func runEnroll(args []string) {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	devices := fs.String("devices", "devices.json", "path to the member registry")
	name := fs.String("name", "", "human name for this machine (required)")
	_ = fs.Parse(args)
	if *name == "" {
		fmt.Fprintln(os.Stderr, "enroll: -name is required")
		os.Exit(2)
	}
	id, secret, err := relay.EnrollDevice(*devices, *name, time.Now().UTC().Format(time.RFC3339))
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
	tlsCert := flag.String("tls-cert", "", "TLS certificate; enables HTTPS/WSS")
	tlsKey := flag.String("tls-key", "", "TLS private key")
	insecure := flag.Bool("insecure", false, "allow plain HTTP off loopback (development only)")
	origins := flag.String("origins", "", "comma-separated allowed Origin hosts; empty means same-host only")
	webDir := flag.String("web-dir", "", "serve the client from this directory instead of the embedded copy")
	trustProxy := flag.Bool("trust-proxy", false, "read X-Forwarded-For for rate limiting and logs (only behind a proxy you control)")
	maxFailures := flag.Int("auth-max-failures", 10, "rejected credentials per address before a lockout; 0 disables")
	flag.Parse()

	if *clientToken == "" {
		log.Fatal("tfrelay: -client-token is required: this relay fronts remote shells")
	}
	tlsOn := *tlsCert != "" && *tlsKey != ""
	if (*tlsCert == "") != (*tlsKey == "") {
		log.Fatal("tfrelay: -tls-cert and -tls-key must be given together")
	}
	if err := validateBinding(*addr, tlsOn, *insecure); err != nil {
		log.Fatalf("tfrelay: %v", err)
	}

	devs, err := relay.LoadDevices(*devices)
	if err != nil {
		log.Fatalf("tfrelay: %v", err)
	}

	var patterns []string
	if *origins != "" {
		patterns = splitAndTrim(*origins)
	}
	r, err := relay.New(relay.Config{
		Devices:         devs,
		ClientToken:     *clientToken,
		OriginPatterns:  patterns,
		TrustProxy:      *trustProxy,
		AuthMaxFailures: *maxFailures,
		Log:             log.Default(),
	})
	if err != nil {
		log.Fatalf("tfrelay: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", r.ServeAgent)
	mux.HandleFunc("/relay/client", r.ServeClient)
	mux.HandleFunc("/relay/machines", r.Machines)
	// The relay serves the same client bundle a daemon does, and tells it which
	// transport to use. Without this a phone could not reach a fleet at all: the
	// relay is the only address that is reachable from outside the LAN.
	mux.HandleFunc("/tf/config", web.Config("relay", "/relay/client"))
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
	log.Printf("tfrelay: %d machine(s) enrolled; listening on %s://%s", len(devs), scheme, *addr)
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
