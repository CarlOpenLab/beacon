package relay_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/CarlOpenLab/term-fleet/internal/relay"
)

// TestEmptyRegistryIsAllowed pins the change that makes the admin setup flow
// possible: a relay whose operator has not enrolled anything yet must still
// start. It used to refuse to boot, which made "deploy, then enrol from the
// panel" impossible in that order.
func TestEmptyRegistryIsAllowed(t *testing.T) {
	if _, err := relay.New(relay.Config{Token: relay.NewTokenStore("tok", "")}); err != nil {
		t.Fatalf("a relay with no machines yet must start: %v", err)
	}
}

// TestEnrollWhileRunningIsVisibleImmediately is the property the panel depends
// on: a credential minted after start is served out of the same registry the
// relay authenticates against, so no restart is involved.
func TestEnrollWhileRunningIsVisibleImmediately(t *testing.T) {
	dir := t.TempDir()
	reg, err := relay.OpenRegistry(filepath.Join(dir, "devices.json"))
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	r, err := relay.New(relay.Config{Registry: reg, Token: relay.NewTokenStore("tok", "")})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}

	if got := r.Roster(); len(got) != 0 {
		t.Fatalf("roster = %v, want empty", got)
	}

	id, secret, err := r.Devices().Enroll("mac-mini", "2026-10-15T00:00:00Z")
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if id == "" || secret == "" {
		t.Fatal("enroll returned empty credentials")
	}

	roster := r.Roster()
	if len(roster) != 1 {
		t.Fatalf("roster = %v, want the new machine", roster)
	}
	if roster[0].ID != id || roster[0].Name != "mac-mini" {
		t.Fatalf("roster[0] = %+v, want id %q named mac-mini", roster[0], id)
	}
	if roster[0].Online {
		t.Fatal("a machine that has not dialled in must not be listed as online")
	}
	if got, ok := r.Devices().Secret(id); !ok || got != secret {
		t.Fatalf("secret = %q,%v, want the issued secret", got, ok)
	}

	// And it survives a reload, so a restart does not lose the machine.
	reopened, err := relay.OpenRegistry(filepath.Join(dir, "devices.json"))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got, ok := reopened.Secret(id); !ok || got != secret {
		t.Fatalf("after reload secret = %q,%v, want it back", got, ok)
	}
}

// TestRevokeWhileRunningRemovesTheMachine is the other half: revoking has to take
// the machine off the roster, not just off disk.
func TestRevokeWhileRunningRemovesTheMachine(t *testing.T) {
	reg, err := relay.OpenRegistry(filepath.Join(t.TempDir(), "devices.json"))
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	r, err := relay.New(relay.Config{Registry: reg, Token: relay.NewTokenStore("tok", "")})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	id, secret, err := r.Devices().Enroll("laptop", "")
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}

	removed, err := r.Devices().Revoke(id)
	if err != nil || !removed {
		t.Fatalf("revoke = %v,%v, want removed", removed, err)
	}
	if _, ok := r.Devices().Secret(id); ok {
		t.Fatal("a revoked machine still authenticates")
	}
	if got := r.Roster(); len(got) != 0 {
		t.Fatalf("roster = %v, want empty after revoke", got)
	}
	_ = secret
}

// TestTokenRotationTakesEffectNow checks that rotating the client token through
// the store is what the relay actually authenticates against. Rotation is only
// worth having if it is immediate and total.
func TestTokenRotationTakesEffectNow(t *testing.T) {
	store := relay.NewTokenStore("first-token", "")
	r, err := relay.New(relay.Config{Token: store})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/client", r.ServeClient)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/client"

	dial := func(token string) (*http.Response, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{
			HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}},
		})
		if conn != nil {
			_ = conn.Close(websocket.StatusNormalClosure, "test done")
		}
		return resp, err
	}

	if _, err := dial("first-token"); err != nil {
		t.Fatalf("the configured token was refused: %v", err)
	}

	next, err := store.Rotate(24 * time.Hour)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if next == "first-token" {
		t.Fatal("rotate returned the old token")
	}

	if resp, err := dial("first-token"); err == nil {
		t.Fatal("the rotated-away token still opens a client socket")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old token got %v, want 401", resp)
	}
	if _, err := dial(next); err != nil {
		t.Fatalf("the new token was refused: %v", err)
	}
}

// TestTokenStoreRoundTripsThroughDisk: a rotation that is not persisted would be
// undone by the next restart, which is the opposite of what rotating is for.
func TestTokenStoreRoundTripsThroughDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.txt")
	s, err := relay.OpenTokenStore(path, "initial")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if s.Get() != "initial" {
		t.Fatalf("token = %q, want the initial value", s.Get())
	}
	if err := s.Set("rotated", 24*time.Hour); err != nil {
		t.Fatalf("set: %v", err)
	}

	again, err := relay.OpenTokenStore(path, "initial")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if again.Get() != "rotated" {
		t.Fatalf("token after reload = %q, want rotated", again.Get())
	}

	// A fresh file with no content must fall back, not come up empty.
	empty := filepath.Join(t.TempDir(), "token.txt")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	fresh, err := relay.OpenTokenStore(empty, "fallback")
	if err != nil {
		t.Fatalf("open empty: %v", err)
	}
	if fresh.Get() != "fallback" {
		t.Fatalf("token = %q, want fallback", fresh.Get())
	}
}

// TestRegistryDeviceRecordShape guards the on-disk format the CLI subcommands
// and the panel share.
func TestRegistryDeviceRecordShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	reg, err := relay.OpenRegistry(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	id, secret, err := reg.Enroll("box", "2026-10-15T00:00:00Z")
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Devices map[string]struct {
			Secret  string `json:"secret"`
			Name    string `json:"name"`
			AddedAt string `json:"addedAt"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatalf("devices.json is not valid JSON: %v", err)
	}
	rec, ok := file.Devices[id]
	if !ok {
		t.Fatalf("devices.json has no entry for %s", id)
	}
	if rec.Secret != secret || rec.Name != "box" || rec.AddedAt == "" {
		t.Fatalf("record = %+v, want the enrolled secret, name and timestamp", rec)
	}
	// Windows has no Unix permission bits: every writable file reports 0666
	// there, so the mode only means something where those bits exist.
	if runtime.GOOS != "windows" {
		if mode := fileMode(t, path); mode != 0o600 {
			t.Fatalf("devices.json mode = %o, want 600: it holds machine credentials", mode)
		}
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st.Mode().Perm()
}
