//go:build !windows

package hub_test

import (
	"context"
	"testing"
	"time"

	"github.com/CarlOpenLab/beacon/internal/hub"
	"github.com/CarlOpenLab/beacon/internal/protocol"
)

// A hub's lifetime bounds its shells' lifetimes. The hub is the only owner of
// the PTYs it opens: a shell that outlives it is a process no client can reach
// and a PTY device the machine cannot hand out again.
//
// macOS allows 511 PTYs in total. A program that starts a hub, opens shells and
// drops the hub — a window reopening, a test case run twenty times — usually
// notices nothing at all, because the leak is invisible until the developer's
// own terminal is refused a PTY ("Your system cannot allocate any more pty
// devices" is Ghostty's version of it). So the promise is worth holding the hub
// to: cancelling the context ends every session it hosts.
func TestCancellingTheHubEndsItsSessions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New(hub.Options{Host: protocol.Host{Hostname: "test", Platform: "test", Arch: "test"}})
	go h.Run(ctx)

	s, err := h.Create("", 80, 24, "", "/bin/sh", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !s.Alive() {
		t.Fatal("a fresh shell is not alive")
	}

	cancel()

	deadline := time.Now().Add(5 * time.Second)
	for s.Alive() {
		if time.Now().After(deadline) {
			t.Fatal("the shell outlived the hub that hosted it: it holds a PTY device nothing can release but this process exiting")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
