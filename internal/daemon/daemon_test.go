package daemon

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/hub"
)

func TestValidateBinding(t *testing.T) {
	cases := []struct {
		name    string
		addr    string
		token   string
		wantErr bool
	}{
		{"loopback without token", "127.0.0.1:7678", "", false},
		{"localhost without token", "localhost:7678", "", false},
		{"ipv6 loopback without token", "[::1]:7678", "", false},
		{"all interfaces without token", "0.0.0.0:7678", "", true},
		{"lan address without token", "192.168.1.20:7678", "", true},
		{"all interfaces with token", "0.0.0.0:7678", "s3cret", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBinding(tc.addr, tc.token)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateBinding(%q, %q) = nil, want an error", tc.addr, tc.token)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateBinding(%q, %q) = %v, want nil", tc.addr, tc.token, err)
			}
		})
	}
}

func TestBaseURLRewritesUnspecifiedHost(t *testing.T) {
	srv, err := New(Options{
		Addr:        "0.0.0.0:0",
		Token:       "s3cret",
		IdleTimeout: time.Minute,
		Client:      http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.Close()

	if got := srv.BaseURL(); !strings.HasPrefix(got, "http://127.0.0.1:") {
		t.Fatalf("BaseURL() = %q, want a 127.0.0.1 URL a local WebView can load", got)
	}
	if !srv.ClientPresent() {
		t.Fatal("ClientPresent() = false, want true when a handler is supplied")
	}
}

// The desktop shell depends on this: the daemon must be answering on its bound
// address by the time the window loads BaseURL, not merely listening.
func TestServeAnswersBeforeWindowLoads(t *testing.T) {
	srv, err := New(Options{
		Addr:   "127.0.0.1:0",
		Client: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "client") }),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := srv.Serve(ctx); err != nil {
			t.Errorf("Serve: %v", err)
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := http.Get(srv.BaseURL() + "/")
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(body) != "client" {
				t.Fatalf("body = %q, want %q", body, "client")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never answered on %s: %v", srv.BaseURL(), err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSplitList(t *testing.T) {
	got := SplitList(" a.example , ,b.example,")
	want := []string{"a.example", "b.example"}
	if len(got) != len(want) {
		t.Fatalf("SplitList = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SplitList = %v, want %v", got, want)
		}
	}
}

// waitForAnswer polls until the server answers, or the deadline passes.
func waitForAnswer(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := http.Get(url + "/")
		if err == nil {
			resp.Body.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never answered on %s: %v", url, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The desktop shell turns its local service off and on while its window keeps
// its shells. Start's stop function must therefore shut the HTTP service down
// without touching the hub it was given, and New must use that hub rather than
// building its own.
func TestStartStopLeavesInjectedHubRunning(t *testing.T) {
	h := hub.New(hub.Options{})
	srv, err := New(Options{
		Addr:   "127.0.0.1:0",
		Hub:    h,
		Client: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "client") }),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if srv.Hub() != h {
		t.Fatal("New built its own hub instead of using the one it was given")
	}

	stop := srv.Start()
	waitForAnswer(t, srv.BaseURL())

	// No shell name: the session should come up on whatever shell the platform
	// treats as its own, which is also the only thing that works on Windows.
	s, err := h.Create("", 80, 24, "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	stop()
	if _, ok := h.Session(s.ID); !ok {
		t.Fatal("stopping the local service ended a session; the hub must outlive it")
	}

	// A second stop is harmless, and the session is still there.
	stop()
	if _, ok := h.Session(s.ID); !ok {
		t.Fatal("a second stop ended a session")
	}
}
