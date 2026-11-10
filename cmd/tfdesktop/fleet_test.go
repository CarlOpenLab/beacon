package main

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func newTestFleet(t *testing.T) *fleet {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	f := newFleet(ctx, log.New(io.Discard, "", 0))
	// A test's shells are real processes on real PTY devices, and a fleet that
	// is merely Closed keeps them: Close drops the listener and the client, and
	// the shells it never asked about go on running until the test binary
	// exits. A case run with -count=20 then holds hundreds of PTYs while the
	// package runs, and the machine has 511 of them in total — enough to leave
	// the developer's own terminal unable to open a window, which is exactly
	// how this was found. Shutting the hub's sessions down is part of tearing a
	// test's fleet down, so do it explicitly rather than leaving it to the
	// cancellation goroutine's scheduling.
	t.Cleanup(func() {
		f.Hub().Manager().Shutdown()
		f.Close()
	})
	t.Cleanup(cancel)
	return f
}

// liveIDs is the set of session ids the hub currently holds.
func liveIDs(t *testing.T, f *fleet) map[string]bool {
	t.Helper()
	ids := make(map[string]bool)
	for _, s := range f.Hub().Sessions() {
		ids[s.SessionID] = true
	}
	return ids
}

// Turning the local service off is not supposed to end anything: the shells
// live in the hub, not in the service, and the window keeps showing them.
func TestServerToggleKeepsSessions(t *testing.T) {
	f := newTestFleet(t)

	if f.ServerOn() {
		t.Fatal("a fresh fleet starts with the local service off, until asked")
	}
	if err := f.SetServer(true); err != nil {
		t.Fatalf("SetServer(true): %v", err)
	}
	if url := f.BaseURL(); url == "" {
		t.Fatal("SetServer(true) reported no address")
	}

	// No shell name, so this exercises the platform's own default, which is the
	// only choice that also works on Windows.
	s, err := f.Hub().Create("", 80, 24, "", "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !liveIDs(t, f)[s.ID] {
		t.Fatalf("session %q missing right after Create", s.ID)
	}

	if err := f.SetServer(false); err != nil {
		t.Fatalf("SetServer(false): %v", err)
	}
	if f.ServerOn() || f.BaseURL() != "" {
		t.Fatal("SetServer(false) left the service running")
	}
	if !liveIDs(t, f)[s.ID] {
		t.Fatal("turning the local service off ended a session; it must not")
	}

	if err := f.SetServer(true); err != nil {
		t.Fatalf("SetServer(true) again: %v", err)
	}
	if !liveIDs(t, f)[s.ID] {
		t.Fatal("turning the local service back on lost the session")
	}
}

func TestSetServerIsIdempotent(t *testing.T) {
	f := newTestFleet(t)
	for i := 0; i < 2; i++ {
		if err := f.SetServer(true); err != nil {
			t.Fatalf("SetServer(true) #%d: %v", i+1, err)
		}
		if err := f.SetServer(false); err != nil {
			t.Fatalf("SetServer(false) #%d: %v", i+1, err)
		}
	}
}

func TestSetRelayRejectsBadURLAndClears(t *testing.T) {
	f := newTestFleet(t)

	// A URL the relay client cannot parse is reported, not retried, so the
	// settings window can say what is wrong.
	if err := f.SetRelay("not-a-url", "dev", "secret", "", true); err == nil {
		t.Fatal("SetRelay accepted a URL that is not a websocket endpoint")
	}
	if f.RelayOn() {
		t.Fatal("a rejected relay left the link marked as on")
	}

	// Disabling, or an empty URL, clears without error.
	if err := f.SetRelay("wss://relay.example/relay/agent", "dev", "secret", "", false); err != nil {
		t.Fatalf("SetRelay(enabled=false): %v", err)
	}
	if err := f.SetRelay("", "dev", "secret", "", true); err != nil {
		t.Fatalf("SetRelay(empty URL): %v", err)
	}
	if f.RelayOn() {
		t.Fatal("RelayOn after clearing")
	}
}

func TestSetRelayRejectsUnusableCAFile(t *testing.T) {
	f := newTestFleet(t)

	// A CA file the shell cannot read is reported before any dial, so the
	// settings window can point at the wrong path instead of failing later
	// with an opaque TLS error.
	if err := f.SetRelay("wss://relay.example/relay/agent", "dev", "secret", "/nonexistent/ca.pem", true); err == nil {
		t.Fatal("SetRelay accepted a CA file that does not exist")
	}
	if f.RelayOn() {
		t.Fatal("a rejected relay left the link marked as on")
	}

	// A file with no PEM certificate in it is rejected too.
	empty := filepath.Join(t.TempDir(), "empty.pem")
	if err := os.WriteFile(empty, []byte("not a certificate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.SetRelay("wss://relay.example/relay/agent", "dev", "secret", empty, true); err == nil {
		t.Fatal("SetRelay accepted a file with no PEM certificate")
	}
}
