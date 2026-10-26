//go:build !windows

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

	"github.com/CarlOpenLab/beacon/internal/agent"
	"github.com/CarlOpenLab/beacon/internal/daemon"
	"github.com/CarlOpenLab/beacon/internal/relay"
	"github.com/CarlOpenLab/beacon/internal/tunnel"
)

// A token minted for one person can be limited to one machine. The relay enforces
// both halves of that: the roster the viewer receives lists only what it may
// reach, and an open for anything else is refused as if it did not exist.
func TestNamedTokenIsScopedToItsMachines(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	rl, err := relay.New(relay.Config{
		Devices: map[string]string{
			"box-a": "secret-a",
			"box-b": "secret-b",
		},
		ClientToken: clientToken,
	})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", rl.ServeAgent)
	mux.HandleFunc("/relay/client", rl.ServeClient)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	// Two daemons, two machines: one process is an artifact of the test, and the
	// hub's instance keeps them apart. Each drops sessions with the test.
	startMember := func(id, secret string) {
		d, err := daemon.New(daemon.Options{Addr: "127.0.0.1:0"})
		if err != nil {
			t.Fatalf("daemon %s: %v", id, err)
		}
		go func() { _ = d.Serve(ctx) }()
		t.Cleanup(func() { _ = d.Close() })
		ag, err := agent.New(agent.Config{
			URL:         wsURL(ts.URL) + "/relay/agent",
			DeviceID:    id,
			Secret:      secret,
			MachineName: id,
			Hub:         d.Hub(),
			MinBackoff:  10 * time.Millisecond,
			MaxBackoff:  50 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("agent %s: %v", id, err)
		}
		go func() { _ = ag.Run(ctx) }()
	}
	startMember("box-a", "secret-a")
	startMember("box-b", "secret-b")

	deadline := time.Now().Add(10 * time.Second)
	for rl.Online() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if rl.Online() < 2 {
		t.Fatalf("members online = %d, want 2", rl.Online())
	}

	n, err := rl.ClientToken().MintNamed("alice's phone", time.Hour, []string{"box-a"})
	if err != nil {
		t.Fatalf("mint named: %v", err)
	}

	dctx, dcancel := context.WithTimeout(ctx, 5*time.Second)
	defer dcancel()
	conn, _, err := websocket.Dial(dctx, wsURL(ts.URL)+"/relay/client", nil)
	if err != nil {
		t.Fatalf("dial client: %v", err)
	}
	defer conn.CloseNow()

	send := func(f tunnel.Frame) {
		b, _ := json.Marshal(f)
		if err := conn.Write(dctx, websocket.MessageText, b); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	read := func() tunnel.Frame {
		_, b, err := conn.Read(dctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var f tunnel.Frame
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return f
	}

	send(tunnel.Frame{T: tunnel.TAuth, Token: n.Token})
	if f := read(); f.T != tunnel.TAuthOK {
		t.Fatalf("auth answer = %+v, want auth.ok", f)
	}
	roster := read()
	if roster.T != tunnel.TMachines {
		t.Fatalf("first frame after auth = %q, want machines", roster.T)
	}
	if len(roster.Machines) != 1 || roster.Machines[0].ID != "box-a" {
		t.Fatalf("roster = %+v, want only box-a", roster.Machines)
	}

	// Opening the machine outside the scope must fail the way an unknown machine
	// does, so the token cannot map the fleet by error.
	send(tunnel.Frame{T: tunnel.TOpen, Machine: "box-b", Stream: 1})
	if f := read(); f.T != tunnel.TError || f.Code != tunnel.ErrNoMachine {
		t.Fatalf("open box-b = %+v, want no_machine", f)
	}

	// And the machine it was granted routes.
	send(tunnel.Frame{T: tunnel.TOpen, Machine: "box-a", Stream: 2})
	if f := read(); f.T != tunnel.TAccept {
		t.Fatalf("open box-a = %+v, want accept", f)
	}

	// Revoking it ends that reach: a second connection cannot authenticate.
	if !rl.ClientToken().RevokeNamed(n.Token) {
		t.Fatal("revoke reported nothing removed")
	}
	conn2, _, err := websocket.Dial(dctx, wsURL(ts.URL)+"/relay/client", nil)
	if err != nil {
		t.Fatalf("dial second client: %v", err)
	}
	defer conn2.CloseNow()
	b, _ := json.Marshal(tunnel.Frame{T: tunnel.TAuth, Token: n.Token})
	_ = conn2.Write(dctx, websocket.MessageText, b)
	_, got, err := conn2.Read(dctx)
	if err == nil && strings.Contains(string(got), `"auth.ok"`) {
		t.Fatalf("a revoked token authenticated: %s", got)
	}
}
