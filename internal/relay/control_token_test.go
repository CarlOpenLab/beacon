package relay_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/CarlOpenLab/term-fleet/internal/relay"
)

// A machine may ask the relay for a short-lived credential instead of being
// handed the fleet's shared secret: it proves it is itself with the device
// credential it already dials in with, and what it gets back is accepted as a
// client until it expires. The shared token stays what a browser is given.
func TestControlTokenIsMintedForEnrolledMachinesOnly(t *testing.T) {
	reg, err := relay.OpenRegistry(filepath.Join(t.TempDir(), "devices.json"))
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	machineID, machineSecret, err := reg.Enroll("mac-mini", "")
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	rl, err := relay.New(relay.Config{
		Registry: reg,
		Token:    relay.NewTokenStore("fleet-token", ""),
		// Short, so the expiry is observable without waiting out a real one.
		ControlTokenTTL: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/client", rl.ServeClient)
	mux.HandleFunc("/relay/control-token", rl.ControlToken)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	clientURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/client"

	mint := func(cred string) (int, string) {
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/relay/control-token", nil)
		if err != nil {
			t.Fatal(err)
		}
		if cred != "" {
			req.Header.Set("Authorization", "Bearer "+cred)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("mint: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp.StatusCode, string(body)
	}

	// The shared token is for viewers, not for machines asking to become one: a
	// browser that holds it must not be able to turn itself into a driver.
	if code, _ := mint("fleet-token"); code != http.StatusUnauthorized {
		t.Fatalf("minting with the shared token got %d, want 401", code)
	}
	if code, _ := mint(""); code != http.StatusUnauthorized {
		t.Fatalf("minting with no credential got %d, want 401", code)
	}
	if code, _ := mint("member-1:wrong"); code != http.StatusUnauthorized {
		t.Fatalf("minting with a wrong secret got %d, want 401", code)
	}
	// A GET is not a mint: the panel's links and a browser's address bar must not
	// be able to hand out credentials.
	get, err := http.Get(ts.URL + "/relay/control-token")
	if err != nil {
		t.Fatal(err)
	}
	get.Body.Close()
	if get.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /relay/control-token got %d, want 405", get.StatusCode)
	}

	code, body := mint(machineID + ":" + machineSecret)
	if code != http.StatusOK {
		t.Fatalf("minting for an enrolled machine got %d: %s", code, body)
	}
	var out struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expiresAt"`
		Device    string `json:"device"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("mint response is not JSON: %v (%s)", err, body)
	}
	if out.Token == "" || out.Token == "fleet-token" {
		t.Fatalf("minted token = %q, want a fresh credential", out.Token)
	}
	if out.Device != machineID {
		t.Errorf("minted token is labelled %q, want %q", out.Device, machineID)
	}
	if exp, err := time.Parse(time.RFC3339, out.ExpiresAt); err != nil {
		t.Errorf("expiresAt = %q: %v", out.ExpiresAt, err)
	} else if !exp.After(time.Now()) {
		t.Errorf("expiresAt = %q, want a time after now (%s)", out.ExpiresAt, time.Now().UTC().Format(time.RFC3339))
	}

	// It is a client credential: the relay accepts it on the client endpoint, and
	// the shared token still works there too.
	if err := dialClient(clientURL, out.Token); err != nil {
		t.Fatalf("the minted credential was refused: %v", err)
	}
	if err := dialClient(clientURL, "fleet-token"); err != nil {
		t.Fatalf("the shared token stopped working: %v", err)
	}
	if err := dialClient(clientURL, "nonsense"); err == nil {
		t.Fatal("the relay accepted a credential it never issued")
	}

	// And it stops on its own: nobody has to notice anything.
	waitForExpiry(t, 5*time.Second, func() bool {
		return dialClient(clientURL, out.Token) != nil
	})

	// Revoking the machine takes away a credential it was just handed, rather
	// than leaving it working until it happens to expire.
	code, body = mint(machineID + ":" + machineSecret)
	if code != http.StatusOK {
		t.Fatalf("re-minting got %d: %s", code, body)
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if err := dialClient(clientURL, out.Token); err != nil {
		t.Fatalf("the second credential was refused: %v", err)
	}
	if _, err := rl.RevokeDevice(machineID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := dialClient(clientURL, out.Token); err == nil {
		t.Fatal("a revoked machine's credential still works")
	}
	if code, _ := mint(machineID + ":" + machineSecret); code != http.StatusUnauthorized {
		t.Fatalf("a revoked machine minted another credential: %d", code)
	}
}

// dialClient returns nil when the relay accepts the credential on the client
// endpoint, and the reason when it does not.
func dialClient(url, token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}},
	})
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "test done")
	}
	return err
}

// waitForExpiry blocks until cond holds, or fails the test.
func waitForExpiry(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the condition never held")
}
