package relay

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The store keeps the newest lines, not the first ones: the end of a log is
// where the thing that just went wrong was written.
func TestLogStoreKeepsTheNewestLines(t *testing.T) {
	s := NewLogStore()
	s.maxLines = 2
	s.Put("mac", LogReport{
		Level: "info",
		Lines: []string{"one", "two", "three", "four"},
	})
	e, ok := s.Get("mac")
	if !ok {
		t.Fatal("the report was not stored")
	}
	if got := strings.Join(e.Lines, ","); got != "three,four" {
		t.Fatalf("lines = %q, want the two newest", got)
	}
	if got := e.Text(); got != "three\nfour" {
		t.Fatalf("text = %q", got)
	}
}

func TestLogStoreReplacesThePreviousReport(t *testing.T) {
	s := NewLogStore()
	s.Put("mac", LogReport{Lines: []string{"old"}})
	s.Put("mac", LogReport{Lines: []string{"new"}})
	e, _ := s.Get("mac")
	if len(e.Lines) != 1 || e.Lines[0] != "new" {
		t.Fatalf("lines = %v, want only the newest report", e.Lines)
	}
	list := s.List()
	if len(list) != 1 || list[0].Lines != 1 {
		t.Fatalf("list = %+v", list)
	}
	if !s.Clear("mac") {
		t.Fatal("clearing a stored report returned false")
	}
	if _, ok := s.Get("mac"); ok {
		t.Fatal("the report survived Clear")
	}
}

// A report is authenticated with the machine's own credential, and the line
// count comes back so a machine can tell a relay that took the body from one
// that silently dropped it.
func TestServeLogRequiresAMachineCredential(t *testing.T) {
	dir := t.TempDir()
	reg, err := OpenRegistry(filepath.Join(dir, "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	id, secret, err := reg.Enroll("mac", time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(Config{Registry: reg, Token: NewTokenStore("client-token", ""), Log: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(r.ServeLog))
	defer ts.Close()

	body, _ := json.Marshal(LogReport{Lines: []string{"hello"}, Level: "debug"})
	post := func(auth string) *http.Response {
		req, _ := http.NewRequest(http.MethodPost, ts.URL, bytes.NewReader(body))
		req.Header.Set("Authorization", auth)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	bad := post("Bearer nope:nope")
	bad.Body.Close()
	if bad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated report = %d, want 401", bad.StatusCode)
	}
	if _, ok := r.Logs().Get(id); ok {
		t.Fatal("an unauthenticated report was stored")
	}

	good := post("Bearer " + id + ":" + secret)
	defer good.Body.Close()
	if good.StatusCode != http.StatusOK {
		t.Fatalf("authenticated report = %d, want 200", good.StatusCode)
	}
	e, ok := r.Logs().Get(id)
	if !ok || len(e.Lines) != 1 || e.Lines[0] != "hello" {
		t.Fatalf("stored report = %+v, %v", e, ok)
	}
}
