package relay

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
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
	if _, err := s.Put("mac", LogReport{
		Level: "info",
		Lines: []string{"one", "two", "three", "four"},
	}); err != nil {
		t.Fatal(err)
	}
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
	if _, err := s.Put("mac", LogReport{Lines: []string{"old"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put("mac", LogReport{Lines: []string{"new"}}); err != nil {
		t.Fatal(err)
	}
	e, _ := s.Get("mac")
	if len(e.Lines) != 1 || e.Lines[0] != "new" {
		t.Fatalf("lines = %v, want only the newest report", e.Lines)
	}
	list := s.List()
	if len(list) != 1 || list[0].Lines != 1 {
		t.Fatalf("list = %+v", list)
	}
	if cleared, err := s.Clear("mac"); err != nil || !cleared {
		t.Fatal("clearing a stored report returned false")
	}
	if _, ok := s.Get("mac"); ok {
		t.Fatal("the report survived Clear")
	}
}

func TestPersistentLogStoreSurvivesRestartAndClearsDisk(t *testing.T) {
	dir := t.TempDir()
	s, err := NewPersistentLogStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := LogReport{Version: "0.7.2", Platform: "windows", Arch: "amd64", Level: "DEBUG", Lines: []string{"attach sent", "machine closed the stream"}}
	if _, err := s.Put("win-2024-e6ecb6", want); err != nil {
		t.Fatal(err)
	}
	if len(s.memory) != 0 {
		t.Fatalf("persistent report retained in memory: %d entry(ies)", len(s.memory))
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("persisted files = %v, %v", files, err)
	}
	info, err := os.Stat(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("report permissions = %04o, want 0600", mode)
	}

	loaded, err := NewPersistentLogStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.Get("win-2024-e6ecb6")
	if !ok || got.Version != want.Version || len(got.Lines) != 2 || got.Lines[1] != "machine closed the stream" {
		t.Fatalf("restored report = %+v, exists=%t", got, ok)
	}
	if cleared, err := loaded.Clear("win-2024-e6ecb6"); err != nil || !cleared {
		t.Fatalf("Clear() = %t, %v", cleared, err)
	}
	files, _ = filepath.Glob(filepath.Join(dir, "*.json"))
	if len(files) != 0 {
		t.Fatalf("persisted report files remain after clear: %v", files)
	}
}

func TestPersistentLogStoreIgnoresCorruptOrUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewPersistentLogStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("loaded corrupt/unrelated data: %+v", got)
	}
}

func TestPersistentLogStoreReportsDiskWriteFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	s, err := NewPersistentLogStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put("mac", LogReport{Lines: []string{"cannot save"}}); err == nil {
		t.Fatal("Put succeeded without being able to persist the report")
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("failed report was indexed as persisted: %+v", got)
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
