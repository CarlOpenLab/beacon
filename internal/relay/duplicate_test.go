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
	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/internal/relay"
	"github.com/CarlOpenLab/beacon/internal/tunnel"
)

// A single daemon can be enrolled twice — two device ids, one machine. Both
// links would then list the same sessions, and each copy could be typed into as
// if it were the other. The relay refuses the second link and names the first,
// so the person removes an enrollment instead of watching two rows fight.
func TestSecondLinkToTheSameDaemonIsRefused(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	d, err := daemon.New(daemon.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("daemon: %v", err)
	}
	go func() { _ = d.Serve(ctx) }()
	t.Cleanup(func() { _ = d.Close() })

	rl, err := relay.New(relay.Config{
		Devices: map[string]string{
			deviceID:   deviceSec,
			"member-2": "member-2-secret",
		},
		ClientToken: clientToken,
	})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", rl.ServeAgent)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	ag, err := agent.New(agent.Config{
		URL:         wsURL(ts.URL) + "/relay/agent",
		DeviceID:    deviceID,
		Secret:      deviceSec,
		MachineName: "first",
		Hub:         d.Hub(),
		MinBackoff:  10 * time.Millisecond,
		MaxBackoff:  50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("agent: %v", err)
	}
	go func() { _ = ag.Run(ctx) }()

	// Wait for the first member to be online.
	deadline := time.Now().Add(10 * time.Second)
	for rl.Online() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if rl.Online() == 0 {
		t.Fatal("the first member never came online")
	}

	hctx, hcancel := context.WithTimeout(ctx, 5*time.Second)
	defer hcancel()
	conn, _, err := websocket.Dial(hctx, wsURL(ts.URL)+"/relay/agent", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer member-2:member-2-secret"}},
	})
	if err != nil {
		t.Fatalf("dial second member: %v", err)
	}
	defer conn.CloseNow()

	hello, _ := json.Marshal(tunnel.Frame{
		T:           tunnel.TAgentHello,
		V:           tunnel.Version,
		MachineID:   "member-2",
		MachineName: "second",
		Host: protocol.Host{
			Hostname: "same-box",
			Platform: "darwin",
			Arch:     "arm64",
			Version:  "0.7.0",
			Instance: d.Hub().Host().Instance,
		},
	})
	if err := conn.Write(hctx, websocket.MessageText, hello); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	_, b, err := conn.Read(hctx)
	if err != nil {
		t.Fatalf("read answer: %v", err)
	}
	var f tunnel.Frame
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("decode answer: %v", err)
	}
	if f.T != tunnel.TError {
		t.Fatalf("the second link was welcomed: %+v", f)
	}
	if !strings.Contains(f.Message, "same as "+deviceID) {
		t.Fatalf("refusal %q does not name the first link", f.Message)
	}
}
