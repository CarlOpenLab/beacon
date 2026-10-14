package main

import (
	"context"
	"io"
	"log"
	"testing"
)

func newTestFleet(t *testing.T) *fleet {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	f := newFleet(ctx, log.New(io.Discard, "", 0))
	t.Cleanup(f.Close)
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

	s, err := f.Hub().Create("", 80, 24, "", "/bin/sh")
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
	if err := f.SetRelay("not-a-url", "dev", "secret", true); err == nil {
		t.Fatal("SetRelay accepted a URL that is not a websocket endpoint")
	}
	if f.RelayOn() {
		t.Fatal("a rejected relay left the link marked as on")
	}

	// Disabling, or an empty URL, clears without error.
	if err := f.SetRelay("wss://relay.example/relay/agent", "dev", "secret", false); err != nil {
		t.Fatalf("SetRelay(enabled=false): %v", err)
	}
	if err := f.SetRelay("", "dev", "secret", true); err != nil {
		t.Fatalf("SetRelay(empty URL): %v", err)
	}
	if f.RelayOn() {
		t.Fatal("RelayOn after clearing")
	}
}
