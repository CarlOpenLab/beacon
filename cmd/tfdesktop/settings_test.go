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
	"runtime"
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
	// Windows has no Unix permission bits: every writable file reports 0666
	// there, so the mode only means something where those bits exist.
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("mode = %v, want 0600", perm)
		}
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

// --- save feedback ---------------------------------------------------------

// A window that has just loaded its settings has nothing to save, and one with
// an edited field does. That flag is what the toolbar shows, so a wrong answer
// here means the window either nags or lies about saving.
func TestSettingsDirtyTracksTheDraftAgainstTheStore(t *testing.T) {
	sw, store := newTestSettingsWindow(t)

	if sw.dirty() {
		t.Fatal("a freshly opened window reports unsaved changes")
	}

	sw.relay = "wss://relay.example/relay/agent"
	if !sw.dirty() {
		t.Fatal("editing the address did not mark the draft as unsaved")
	}

	sw.commit()
	if sw.dirty() {
		t.Fatal("a committed draft still reports unsaved changes")
	}
	if got := store.Get().RelayURL; got != "wss://relay.example/relay/agent" {
		t.Fatalf("stored RelayURL = %q, want the applied value", got)
	}

	// Whitespace that only the store's own trimming would remove is not a
	// pending change: otherwise the marker would never go away.
	sw.relay = "  " + store.Get().RelayURL + "  "
	if sw.dirty() {
		t.Error("padding the address with spaces counts as an unsaved change")
	}
}

// The toolbar is where the user looks after clicking 应用, so both halves of
// the feedback have to actually reach the screen.
func TestSettingsToolbarShowsPendingChangesAndClearsThem(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()

	if _, ok := tt.Find("● 有未保存的改动"); ok {
		t.Fatal("the pending-changes marker is shown before anything was edited")
	}

	sw.relay = "wss://relay.example/relay/agent"
	tt.Frame()
	if _, ok := tt.Find("● 有未保存的改动"); !ok {
		t.Fatalf("no pending-changes marker after editing; texts = %v", tt.Texts())
	}

	if err := tt.Click("应用"); err != nil {
		t.Fatalf("Click(应用): %v", err)
	}
	tt.Frame()
	if _, ok := tt.Find("● 有未保存的改动"); ok {
		t.Fatalf("the pending-changes marker survived 应用; texts = %v", tt.Texts())
	}
}

// The status line is the only confirmation the click worked, so it has to name
// the time it happened and be distinguishable from a failure.
func TestSettingsStatusReportsWhenItSaved(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	sw.commit()

	if !strings.Contains(sw.status, "已保存") {
		t.Fatalf("status = %q, want it to say the settings were saved", sw.status)
	}
	if !strings.Contains(sw.status, sw.savedAt.Format("15:04:05")) {
		t.Errorf("status = %q, want it to carry the time %s", sw.status, sw.savedAt.Format("15:04:05"))
	}
	if sw.statusErr {
		t.Error("a successful commit is marked as a failure")
	}
	if sw.savedAt.IsZero() {
		t.Error("a successful commit did not record when it happened")
	}
}

func TestSettingsStatusMarksFailures(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	// An unusable CA path is one of the ways applying the draft fails.
	sw.ca = filepath.Join(t.TempDir(), "missing.pem")
	sw.relay = "wss://relay.example/relay/agent"
	sw.relayOn = true
	sw.commit()

	if !sw.statusErr {
		t.Fatalf("status = %q, want it marked as a failure", sw.status)
	}

	// And the line must look different from a save once it is on screen.
	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()
	if err := tt.Click("远程"); err != nil {
		t.Fatalf("Click(远程): %v", err)
	}
	tt.Frame()
	if _, ok := tt.Find(sw.status); !ok {
		t.Errorf("the failure message is not rendered; texts = %v", tt.Texts())
	}
}

// --- font picker -----------------------------------------------------------

// The font list has to include what is actually installed. A curated list
// cannot: Nerd Font builds are named after the family they patch, so a machine
// can have "Maple Mono NF" while the field only offers "Maple Mono".
func TestSettingsFontChoicesIncludeInstalledFamilies(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	sw.installedMu.Lock()
	sw.installed = []string{"Maple Mono NF", "Maple Mono", ""}
	sw.installedMu.Unlock()

	choices := sw.familyChoices()
	if !contains(choices, "Maple Mono NF") {
		t.Fatalf("an installed Nerd Font is not offered: %v", choices)
	}
	if choices[0] != systemFamily {
		t.Errorf("choices start with %q, want %q first", choices[0], systemFamily)
	}

	// "Maple Mono" is both installed and well known; it must appear once.
	seen := map[string]int{}
	for _, name := range choices {
		seen[name]++
	}
	for name, n := range seen {
		if n > 1 {
			t.Errorf("%q is offered %d times", name, n)
		}
	}
	if seen[""] != 0 {
		t.Error("an empty family name reached the suggestions")
	}
}

// The field is free text, so a family that no list mentions still has to work:
// this is the path the report was about.
func TestSettingsFontFieldAcceptsAnUnlistedFamily(t *testing.T) {
	sw, store := newTestSettingsWindow(t)
	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()

	rect, ok := tt.Find("字体")
	if !ok {
		t.Fatalf("the font field is missing; texts = %v", tt.Texts())
	}
	// Start from an empty field so the caret position cannot change what is
	// typed into it.
	sw.family = ""
	tt.Frame()
	// The label sits above its input, as in the other panes.
	tt.ClickAt(rect.X+40, rect.Y+rect.H+12)
	tt.Type("Maple Mono NF")
	tt.Frame()

	if sw.family != "Maple Mono NF" {
		t.Fatalf("typed family = %q, want it in the draft", sw.family)
	}
	if err := tt.Click("应用"); err != nil {
		t.Fatalf("Click(应用): %v", err)
	}
	tt.Frame()

	if got := store.Get().FontFamily; got != "Maple Mono NF" {
		t.Errorf("stored FontFamily = %q, want the typed family", got)
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
