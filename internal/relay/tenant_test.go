package relay_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/CarlOpenLab/beacon/internal/relay"
	"github.com/CarlOpenLab/beacon/internal/tunnel"
)

// fakeUsers stands in for the accounts store: it maps a bearer token to the
// tenant that token may reach.
type fakeUsers map[string]string

func (f fakeUsers) VerifyLoginToken(tok string) (tenant, user string, ok bool) {
	tenant, ok = f[tok]
	return tenant, "user-of-" + tenant, ok
}

// wsURLTenant is the test server's websocket address (a local copy: the helper
// in scope_test.go is excluded on Windows).
func wsURLTenant(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}

// setupTenantRelay builds a relay on a real registry holding three machines:
// one for alice's tenant, one for bob's, one for the operator's own scope.
// The returned map gives each machine's actual id — the registry suffixes ids
// with random hex, so tests never hard-code them.
func setupTenantRelay(t *testing.T) (*relay.Relay, *httptest.Server, map[string]string) {
	t.Helper()
	reg, err := relay.OpenRegistry(t.TempDir() + "/devices.json")
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	ids := map[string]string{}
	for _, e := range []struct{ name, tenant string }{
		{"box-a", "u-alice"},
		{"box-b", "u-bob"},
		{"box-c", ""},
	} {
		id, _, err := reg.Enroll(e.name, e.tenant, "2026-10-30T00:00:00Z")
		if err != nil {
			t.Fatalf("enroll %s: %v", e.name, err)
		}
		ids[e.name] = id
	}
	rl, err := relay.New(relay.Config{
		Registry:    reg,
		ClientToken: "shared-secret",
	})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	rl.ClientToken().SetLoginVerifier(fakeUsers{
		"alice-token": "u-alice",
		"bob-token":   "u-bob",
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", rl.ServeAgent)
	mux.HandleFunc("/relay/client", rl.ServeClient)
	mux.HandleFunc("/relay/machines", rl.Machines)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return rl, ts, ids
}

// rosterOf asks the JSON endpoint what a bearer credential may see.
func rosterOf(t *testing.T, ts *httptest.Server, token string) []string {
	t.Helper()
	req, err := http.NewRequest("GET", ts.URL+"/relay/machines", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("machines: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("machines with %q: HTTP %d", token, res.StatusCode)
	}
	var body struct {
		Machines []struct {
			ID string `json:"id"`
		} `json:"machines"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	ids := make([]string, 0, len(body.Machines))
	for _, m := range body.Machines {
		ids = append(ids, m.ID)
	}
	return ids
}

// A user login token sees its own tenant's machines and nothing else — not the
// other tenants', and not the operator's tenant-less machines either.
func TestTenantTokenSeesOnlyItsOwnMachines(t *testing.T) {
	_, ts, ids := setupTenantRelay(t)

	if got := rosterOf(t, ts, "alice-token"); len(got) != 1 || got[0] != ids["box-a"] {
		t.Fatalf("alice's roster = %v, want [%s]", got, ids["box-a"])
	}
	if got := rosterOf(t, ts, "bob-token"); len(got) != 1 || got[0] != ids["box-b"] {
		t.Fatalf("bob's roster = %v, want [%s]", got, ids["box-b"])
	}
	// The shared token is the operator's: it reaches every tenant and the
	// tenant-less scope.
	if got := rosterOf(t, ts, "shared-secret"); len(got) != 3 {
		t.Fatalf("shared-token roster = %v, want all three", got)
	}
}

// The socket behaves like the JSON endpoint: a filtered roster on entry, and an
// open for a machine outside the tenant refused exactly the way an unknown
// machine is, so the error cannot map the fleet.
func TestTenantSocketIsolation(t *testing.T) {
	_, ts, ids := setupTenantRelay(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURLTenant(ts.URL)+"/relay/client", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()

	send := func(f tunnel.Frame) {
		b, _ := json.Marshal(f)
		if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	read := func() tunnel.Frame {
		_, b, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var f tunnel.Frame
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return f
	}

	send(tunnel.Frame{T: tunnel.TAuth, Token: "alice-token"})
	if f := read(); f.T != tunnel.TAuthOK {
		t.Fatalf("auth answer = %+v, want auth.ok", f)
	}
	roster := read()
	if roster.T != tunnel.TMachines {
		t.Fatalf("first frame = %q, want machines", roster.T)
	}
	if len(roster.Machines) != 1 || roster.Machines[0].ID != ids["box-a"] {
		t.Fatalf("roster = %+v, want only %s", roster.Machines, ids["box-a"])
	}

	// Another tenant's machine and the operator's own are both "no such machine".
	for _, target := range []string{ids["box-b"], ids["box-c"]} {
		send(tunnel.Frame{T: tunnel.TOpen, Machine: target, Stream: 1})
		if f := read(); f.T != tunnel.TError || f.Code != tunnel.ErrNoMachine {
			t.Fatalf("open %s = %+v, want no_machine", target, f)
		}
	}
}

// A machine's own control credential carries its tenant: it may drive the rest
// of its fleet, and nothing outside it.
func TestControlTokenStaysInItsTenant(t *testing.T) {
	rl, ts, ids := setupTenantRelay(t)

	token, _, err := rl.ClientToken().Mint(ids["box-a"], "u-alice", time.Minute)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if got := rosterOf(t, ts, token); len(got) != 1 || got[0] != ids["box-a"] {
		t.Fatalf("box-a's control roster = %v, want [%s]", got, ids["box-a"])
	}

	// A machine with no tenant keeps the operator-wide reach it always had.
	opToken, _, err := rl.ClientToken().Mint(ids["box-c"], "", time.Minute)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if got := rosterOf(t, ts, opToken); len(got) != 3 {
		t.Fatalf("tenant-less control roster = %v, want all three", got)
	}
}

// Enrolling after a token was minted cannot widen its reach: the tenant check
// reads the live registry, so a machine enrolled into another tenant stays
// invisible to this credential.
func TestRosterFollowsLaterEnrollment(t *testing.T) {
	rl, ts, ids := setupTenantRelay(t)

	// The verifier says "u-alice"; box-d joins alice's tenant mid-flight.
	// The roster grows; bob's token learns nothing about it.
	if _, _, err := rl.Devices().Enroll("box-d", "u-alice", "2026-10-30T01:00:00Z"); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if got := rosterOf(t, ts, "alice-token"); len(got) != 2 {
		t.Fatalf("alice's roster = %v, want box-a and box-d", got)
	}
	if got := strings.Join(rosterOf(t, ts, "bob-token"), ","); got != ids["box-b"] {
		t.Fatalf("bob's roster = %v, want just box-b", got)
	}
}
