package main

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/agent"
	"github.com/CarlOpenLab/term-fleet/internal/settings"
)

// newTestSettingsWindow opens the settings window over a throwaway store. The
// window handle is a zero value on purpose: reletter returns early while there
// is no real window behind it, which is what a headless view test wants.
func newTestSettingsWindow(t *testing.T) (*settingsWindow, *settings.Store) {
	t.Helper()
	f := newTestFleet(t)
	store := settings.Open(t.TempDir())
	return newSettingsWindow(&window{}, store, f), store
}

// The three panes are what the request was about: the terminal's font, the
// remote channel, and the local service switch.
func TestSettingsPanesRenderAndSwitch(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()

	if !tt.HasText("字体") || !tt.HasText("字号") {
		t.Fatalf("the terminal pane is missing its fields: %q", tt.Texts())
	}

	if err := tt.Click("远程"); err != nil {
		t.Fatalf("Click(远程): %v", err)
	}
	tt.Frame()
	if !tt.HasText("公网地址") || !tt.HasText("接入") {
		t.Fatalf("the remote pane did not open: %q", tt.Texts())
	}

	if err := tt.Click("服务"); err != nil {
		t.Fatalf("Click(服务): %v", err)
	}
	tt.Frame()
	if !tt.HasText("本机服务") {
		t.Fatalf("the server pane did not open: %q", tt.Texts())
	}
}

func TestSettingsRestoreDefaultsResetsTheDraft(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	sw.family = "Menlo"
	sw.size = 22
	sw.relay = "wss://relay.example/relay/agent"
	sw.device = "dev"
	sw.relayOn = true

	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()
	if err := tt.Click("恢复默认"); err != nil {
		t.Fatalf("Click(恢复默认): %v", err)
	}
	tt.Frame()

	if sw.family != systemFamily {
		t.Errorf("family after restore = %q, want %q", sw.family, systemFamily)
	}
	if sw.size != settings.DefaultFontSize {
		t.Errorf("size after restore = %v, want %v", sw.size, settings.DefaultFontSize)
	}
	if sw.relay != "" || sw.device != "" || sw.secret != "" {
		t.Errorf("relay draft after restore = %q/%q/%q, want empty", sw.relay, sw.device, sw.secret)
	}
	if sw.relayOn {
		t.Error("restore left the relay marked as on")
	}
	if !sw.server {
		t.Error("restore left the local service off")
	}
}

// 应用 is what commits the text fields; a half-typed address must not reach the
// store until it is pressed.
func TestSettingsApplyWritesTheStore(t *testing.T) {
	sw, store := newTestSettingsWindow(t)
	sw.relay = "  wss://relay.example/relay/agent  "
	sw.device = " dev "
	sw.secret = " sec "
	sw.size = 18
	sw.relayOn = false

	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()
	if err := tt.Click("应用"); err != nil {
		t.Fatalf("Click(应用): %v", err)
	}
	tt.Frame()

	got := store.Get()
	if got.RelayURL != "wss://relay.example/relay/agent" {
		t.Errorf("stored RelayURL = %q, want it trimmed", got.RelayURL)
	}
	if got.DeviceID != "dev" || got.DeviceSecret != "sec" {
		t.Errorf("stored credential = %q/%q, want trimmed", got.DeviceID, got.DeviceSecret)
	}
	if got.FontSize != 18 {
		t.Errorf("stored FontSize = %v, want 18", got.FontSize)
	}
	if got.ServerOff {
		t.Error("the local service was left off although its switch is on")
	}
	if sw.status == "" || strings.HasPrefix(sw.status, "设置没保存") {
		t.Errorf("status after apply = %q, want a success summary", sw.status)
	}
}

// The remote pane's three fields have to be real text inputs, not in-place
// editable labels: clicking them and typing must reach the draft. This guards
// a regression where they were drawn with EditableText, so the pane showed a
// label with no input box under it.
func TestSettingsRemoteFieldsAcceptTyping(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()
	if err := tt.Click("远程"); err != nil {
		t.Fatalf("Click(远程): %v", err)
	}
	tt.Frame()

	cases := []struct {
		label string
		typed string
		draft func() string
	}{
		{"公网地址", "wss://relay.example/relay/agent", func() string { return sw.relay }},
		{"设备 ID", "mac-mini", func() string { return sw.device }},
		{"设备密钥", "s3cret", func() string { return sw.secret }},
	}
	for _, tc := range cases {
		rect, ok := tt.Find(tc.label)
		if !ok {
			t.Fatalf("Find(%q): the field label is missing from the pane", tc.label)
		}
		// The label sits above its input, so aim just below the label's box.
		tt.ClickAt(rect.X+40, rect.Y+rect.H+12)
		tt.Type(tc.typed)
		tt.Frame()
		if got := tc.draft(); got != tc.typed {
			t.Errorf("%s: typed %q but the draft holds %q", tc.label, tc.typed, got)
		}
	}
}

// testCertPEM returns a certificate in the shape the desktop has to cope with:
// a real one, issued for a host rather than by a public CA.
func testCertPEM(t *testing.T) ([]byte, *x509.Certificate) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)
	cert := srv.Certificate()
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), cert
}

func TestSettingsRemotePaneOffersCertHelpers(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()
	if err := tt.Click("远程"); err != nil {
		t.Fatalf("Click(远程): %v", err)
	}
	tt.Frame()

	// Downloading a certificate from the admin page and then finding its path is
	// the friction these buttons exist to remove, so both must be visible
	// without filling anything in first.
	for _, want := range []string{"从服务器获取", "选择文件…"} {
		if _, ok := tt.Find(want); !ok {
			t.Errorf("%q button is missing from the remote pane", want)
		}
	}
	if _, ok := tt.Find("清除"); ok {
		t.Error("清除 should only appear once a certificate is set")
	}
}

func TestSettingsCANoteDescribesThePinnedCertificate(t *testing.T) {
	sw, store := newTestSettingsWindow(t)
	pemBytes, _ := testCertPEM(t)
	path := filepath.Join(t.TempDir(), "relay-ca.pem")
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	// Describe the file the same way the window does, so the assertion is about
	// the note being present rather than about a second, independent guess at
	// what the subject reads like.
	subject, err := agent.DescribeCert(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	if subject.Subject == "" {
		t.Fatal("test certificate has no subject to look for")
	}

	sw.ca = path
	sw.commit()

	if !strings.Contains(sw.caNote, subject.Subject) {
		t.Errorf("note = %q, want it to name %q", sw.caNote, subject.Subject)
	}
	// The path must survive into the file the app actually reads on startup: a
	// note that describes the draft while the store holds something else would
	// be worse than no note.
	if got := store.Get().RelayCA; got != path {
		t.Errorf("stored RelayCA = %q, want %q", got, path)
	}

	// ...and the note has to be on screen, not merely in a field: a summary
	// nobody can read is the same as no feedback at all.
	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()
	if err := tt.Click("远程"); err != nil {
		t.Fatalf("Click(远程): %v", err)
	}
	tt.Frame()
	// Find needs the whole element text, so assert on the note verbatim: the
	// property under test is that what the window holds reaches the screen. The
	// Contains check above already pins down what the note says.
	if _, ok := tt.Find(sw.caNote); !ok {
		t.Errorf("the certificate note is not rendered in the remote pane; texts = %v", tt.Texts())
	}
	if _, ok := tt.Find("清除"); !ok {
		t.Error("清除 should appear once a certificate is set")
	}
}

func TestSettingsCANoteExplainsAnUnusableFile(t *testing.T) {
	cases := []struct {
		name string
		path func(t *testing.T) string
		want string
	}{
		{
			name: "missing",
			path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "nope.pem") },
			want: "读不到",
		},
		{
			name: "not a certificate",
			path: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "junk.pem")
				if err := os.WriteFile(p, []byte("hello"), 0o600); err != nil {
					t.Fatal(err)
				}
				return p
			},
			want: "不是有效的证书",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sw, _ := newTestSettingsWindow(t)
			sw.ca = tc.path(t)
			sw.commit()
			if !strings.Contains(sw.caNote, tc.want) {
				t.Errorf("note = %q, want it to mention %q", sw.caNote, tc.want)
			}
		})
	}
}

func TestSettingsFetchCAWithoutAddressGuidesInsteadOfFailingQuietly(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	sw.relay = ""
	sw.fetchCA()
	if !strings.Contains(sw.caNote, "公网地址") {
		t.Errorf("note = %q, want it to point at the address field", sw.caNote)
	}
}

func TestFetchAndPinCASavesTheCertificateTheRelayPresents(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "https://")

	dir := filepath.Join(t.TempDir(), "config")
	path, err := fetchAndPinCA("wss://"+addr+"/relay/agent", dir, 10*time.Second)
	if err != nil {
		t.Fatalf("fetchAndPinCA: %v", err)
	}
	if path != filepath.Join(dir, "relay-ca.pem") {
		t.Errorf("path = %q, want it under the config dir", path)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the certificate was not written: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %v, want 0600", perm)
	}

	// What was pinned must be the certificate the relay actually showed,
	// otherwise the button hands the app a file that cannot authenticate
	// anything.
	pinned, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := agent.DescribeCert(pinned)
	if err != nil {
		t.Fatalf("the saved file is not a certificate: %v", err)
	}
	sum := sha256.Sum256(srv.Certificate().Raw)
	if want := hex.EncodeToString(sum[:]); got.Fingerprint != want {
		t.Errorf("pinned fingerprint = %s, want %s", got.Fingerprint, want)
	}
}
