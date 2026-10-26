package relay_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CarlOpenLab/beacon/internal/relay"
	"github.com/coder/websocket"
)

// TestTokenExpirySurvivesRestart: a lifetime that did not survive a restart
// would hand back the previous token, which is the one thing expiry is for.
func TestTokenExpirySurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.txt")
	s, err := relay.OpenTokenStore(path, "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.Set("rotated", 90*time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	want := s.ExpiresAt()
	if want.IsZero() {
		t.Fatal("a token set with a lifetime reports no expiry")
	}

	again, err := relay.OpenTokenStore(path, "")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := again.ExpiresAt()
	if got.IsZero() {
		t.Fatal("the expiry was lost on reload")
	}
	if d := got.Sub(want); d > time.Second || d < -time.Second {
		t.Fatalf("expiry after reload = %s, want about %s", got, want)
	}
}

// TestTokenWithoutLifetimeNeverExpires: the zero value has to mean "forever",
// because that is what a token passed on the command line has always meant.
func TestTokenWithoutLifetimeNeverExpires(t *testing.T) {
	s := relay.NewTokenStore("fixed-token", "")
	if !s.ExpiresAt().IsZero() {
		t.Fatalf("expiry = %s, want none", s.ExpiresAt())
	}
	if s.Expired() {
		t.Fatal("a token with no expiry reports itself expired")
	}
	if s.TTL() != 0 {
		t.Fatalf("ttl = %s, want 0", s.TTL())
	}
}

// TestLegacyTokenFileStillLoads: relays deployed before expiry existed wrote the
// bare token. Reading one must not lock existing viewers out.
func TestLegacyTokenFileStillLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.txt")
	if err := os.WriteFile(path, []byte("legacy-token\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s, err := relay.OpenTokenStore(path, "from-flag")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if s.Get() != "legacy-token" {
		t.Fatalf("token = %q, want the legacy value", s.Get())
	}
	if !s.ExpiresAt().IsZero() {
		t.Fatalf("legacy token got expiry %s, want none", s.ExpiresAt())
	}
}

// TestCorruptExpiryFailsClosed: a file we cannot date must not be read as an
// eternal credential.
func TestCorruptExpiryFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.txt")
	body := `{"token":"half-written","expiresAt":"not-a-date"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s, err := relay.OpenTokenStore(path, "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !s.Expired() {
		t.Fatal("a token with an unreadable expiry is still accepted")
	}
}

// TestExpiredTokenIsRefusedOverTheWire: expiry has to be enforced at the point
// where a viewer is admitted, not merely reported by the store.
func TestExpiredTokenIsRefusedOverTheWire(t *testing.T) {
	store := relay.NewTokenStore("good-token", "")
	if err := store.Set("good-token", 60*time.Millisecond); err != nil {
		t.Fatalf("set: %v", err)
	}
	r, err := relay.New(relay.Config{Token: store})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/client", r.ServeClient)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/client"

	dial := func() (*http.Response, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{
			HTTPHeader: http.Header{"Authorization": []string{"Bearer good-token"}},
		})
		if conn != nil {
			_ = conn.Close(websocket.StatusNormalClosure, "test done")
		}
		return resp, err
	}

	if _, err := dial(); err != nil {
		t.Fatalf("the fresh token was refused: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !store.Expired() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	resp, err := dial()
	if err == nil {
		t.Fatal("an expired token still opens a client socket")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired token got %v, want 401", resp)
	}
}
