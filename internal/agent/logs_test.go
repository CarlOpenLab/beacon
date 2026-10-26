package agent

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CarlOpenLab/beacon/internal/diag"
)

// The log endpoint is the member endpoint's sibling, and the scheme follows it:
// a relay behind TLS is posted to over HTTPS.
func TestLogEndpointFollowsTheMemberEndpoint(t *testing.T) {
	cases := []struct{ in, want string }{
		{"wss://relay.example/relay/agent", "https://relay.example/relay/log"},
		{"ws://127.0.0.1:7698/relay/agent", "http://127.0.0.1:7698/relay/log"},
		{"wss://relay.example/relay/agent/", "https://relay.example/relay/log"},
		{"wss://relay.example/custom", "https://relay.example/custom/log"},
	}
	for _, c := range cases {
		a := &Agent{cfg: Config{URL: c.in}}
		if got := a.logEndpoint(); got != c.want {
			t.Errorf("logEndpoint(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The reporter pushes the tail with the machine credential, and a quiet log is
// not uploaded twice.
func TestReportLogsPushesTheTail(t *testing.T) {
	var mu sync.Mutex
	var got []logReport
	var auth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		var report logReport
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			t.Errorf("decode: %v", err)
		}
		mu.Lock()
		got = append(got, report)
		mu.Unlock()
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer ts.Close()

	logger := diag.New(diag.Info)
	logger.Logf(diag.Info, "term", "one")
	a := &Agent{
		cfg: Config{
			URL:         "ws://relay.example/relay/agent",
			LogURL:      ts.URL,
			DeviceID:    "mac-1",
			Secret:      "secret",
			Diag:        logger,
			LogInterval: 5 * time.Millisecond,
			Log:         log.New(io.Discard, "", 0),
		},
		log: log.New(io.Discard, "", 0),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.reportLogs(ctx); close(done) }()
	time.Sleep(35 * time.Millisecond)
	logger.Logf(diag.Info, "term", "two")
	time.Sleep(35 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 {
		t.Fatal("no log report was uploaded")
	}
	if auth != "Bearer mac-1:secret" {
		t.Fatalf("Authorization = %q", auth)
	}
	last := got[len(got)-1]
	if len(last.Lines) < 2 || !strings.Contains(last.Lines[len(last.Lines)-1], "two") {
		t.Fatalf("last report = %+v, want the newest line", last)
	}
	if last.Level != "INFO" {
		t.Fatalf("level = %q, want INFO", last.Level)
	}
}
