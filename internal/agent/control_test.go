package agent_test

import (
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/agent"
)

// The endpoint is derived from the address a machine already joins with, so
// nobody has to know a second URL.
func TestControlTokenURLSitsBesideTheMemberEndpoint(t *testing.T) {
	cases := []struct{ member, want string }{
		{"wss://relay.example/relay/agent", "https://relay.example/relay/control-token"},
		{"ws://127.0.0.1:7698/relay/agent", "http://127.0.0.1:7698/relay/control-token"},
		{"wss://relay.example/fleet/agent", "https://relay.example/fleet/control-token"},
	}
	for _, tc := range cases {
		got, err := agent.ControlTokenURL(tc.member)
		if err != nil {
			t.Errorf("ControlTokenURL(%q): %v", tc.member, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ControlTokenURL(%q) = %q, want %q", tc.member, got, tc.want)
		}
	}

	// An address with no endpoint cannot say where the relay's other endpoints
	// are, and guessing would send a credential somewhere nobody meant.
	for _, bad := range []string{"", "https://relay.example/relay/agent", "wss://relay.example", "not a url"} {
		if got, err := agent.ControlTokenURL(bad); err == nil {
			t.Errorf("ControlTokenURL(%q) = %q, want an error", bad, got)
		}
	}
}

// The whole call: a machine proves itself with its device credential and gets
// back a credential with a lifetime, and every refusal is a reason it can show.
func TestRequestControlTokenCarriesTheDeviceCredential(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch {
		case r.URL.Path != "/relay/control-token":
			w.WriteHeader(http.StatusNotFound)
		case gotAuth != "Bearer member-1:member-1-secret":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized"))
		default:
			expires := time.Now().Add(15 * time.Minute).UTC().Format(time.RFC3339)
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "minted-token", "expiresAt": expires})
		}
	}))
	defer srv.Close()
	member := "ws" + strings.TrimPrefix(srv.URL, "http") + "/relay/agent"

	if _, err := agent.RequestControlToken(member, "", "secret", nil, 5*time.Second); err == nil {
		t.Fatal("asking with no device id was allowed")
	}

	token, err := agent.RequestControlToken(member, "member-1", "member-1-secret", nil, 5*time.Second)
	if err != nil {
		t.Fatalf("RequestControlToken: %v", err)
	}
	if token.Token != "minted-token" {
		t.Errorf("token = %q, want the relay's answer", token.Token)
	}
	if !token.ExpiresAt.After(time.Now().Add(14 * time.Minute)) {
		t.Errorf("expiry = %v, want the lifetime the relay sent", token.ExpiresAt)
	}

	// A machine the relay does not know gets an answer a person can act on
	// rather than a bare 401.
	if _, err := agent.RequestControlToken(member, "member-1", "wrong", nil, 5*time.Second); err == nil {
		t.Fatal("a wrong device secret was accepted")
	} else if !strings.Contains(err.Error(), "enroll") {
		t.Errorf("error = %q, want it to say the machine is not enrolled", err)
	}
}

// A relay behind a private CA is trusted the same way the member link trusts it:
// by the pinned certificate, not by the system store.
func TestRequestControlTokenTrustsThePinnedCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token":     "minted-token",
			"expiresAt": time.Now().Add(time.Minute).UTC().Format(time.RFC3339),
		})
	}))
	defer srv.Close()
	member := "wss" + strings.TrimPrefix(srv.URL, "https") + "/relay/agent"

	// Without the pin, the certificate is not one the system store knows.
	if _, err := agent.RequestControlToken(member, "member-1", "sec", nil, 5*time.Second); err == nil {
		t.Fatal("a self-signed relay was trusted without a pinned certificate")
	}

	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	token, err := agent.RequestControlToken(member, "member-1", "sec", pool, 5*time.Second)
	if err != nil {
		t.Fatalf("RequestControlToken with a pinned certificate: %v", err)
	}
	if token.Token != "minted-token" {
		t.Errorf("token = %q, want the relay's answer", token.Token)
	}
}
