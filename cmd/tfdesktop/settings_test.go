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

	"github.com/CarlOpenLab/beacon/internal/agent"
	"github.com/CarlOpenLab/beacon/internal/relay"
	"github.com/CarlOpenLab/beacon/internal/settings"
	"github.com/CarlOpenLab/beacon/internal/tunnel"
	"github.com/CarlOpenLab/beacon/locales"
)

// newTestSettingsWindow opens the settings window over a throwaway store. The
// window handle is a zero value on purpose: reletter returns early while there
// is no real window behind it, which is what a headless view test wants.
func newTestSettingsWindow(t *testing.T) (*settingsWindow, *settings.Store) {
	t.Helper()
	f := newTestFleet(t)
	store := settings.Open(t.TempDir())
	return newSettingsWindow(&window{store: store}, store, f), store
}

// The three panes are what the request was about: the terminal's font, the
// remote channel, and the local service switch.
func TestSettingsPanesRenderAndSwitch(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()

	if !tt.HasText(locales.SettingsFont()) || !tt.HasText(locales.SettingsFontSize()) {
		t.Fatalf("the terminal pane is missing its fields: %q", tt.Texts())
	}

	if err := tt.Click(locales.SettingsPaneRelay()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsPaneRelay(), err)
	}
	tt.Frame()
	if !tt.HasText(locales.SettingsRelayUrl()) || !tt.HasText(locales.SettingsRelayOn()) {
		t.Fatalf("the remote pane did not open: %q", tt.Texts())
	}

	if err := tt.Click(locales.SettingsPaneServer()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsPaneServer(), err)
	}
	tt.Frame()
	if !tt.HasText(locales.SettingsServer()) {
		t.Fatalf("the server pane did not open: %q", tt.Texts())
	}
	// The firewall prompt is named before it happens. macOS shows it the first
	// time a listening socket appears and offers no way for an app to
	// pre-authorise itself, so the only thing that can be done is tell the
	// person what the prompt is and that clicking 允许 settles it for good.
	if !tt.HasText(locales.SettingsServerDesc()) {
		t.Fatalf("the server pane does not explain the macOS firewall prompt: %q", tt.Texts())
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
	if err := tt.Click(locales.SettingsRestore()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsRestore(), err)
	}
	tt.Frame()

	if sw.family != systemFamily() {
		t.Errorf("family after restore = %q, want %q", sw.family, systemFamily())
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
	if err := tt.Click(locales.SettingsApply()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsApply(), err)
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
	if sw.status == "" || strings.HasPrefix(sw.status, locales.SettingsSaveFailed("")) {
		t.Errorf("status after apply = %q, want a success summary", sw.status)
	}
}

// The language is a segmented control of the platform's look, and a click
// applies the language at once: no 应用 to look for.
func TestTheLanguageSegmentedControlAppliesOnAClick(t *testing.T) {
	sw, store := newTestSettingsWindow(t)
	sw.pane = paneTerminal
	tt := ui.NewTester(sw.view, 760, 600)
	tt.Frame()

	if err := tt.Click(locales.SettingsLangEn()); err != nil {
		t.Fatalf("click English: %v", err)
	}
	tt.Frame()
	if got := store.Get().Language; got != "en" {
		t.Errorf("stored Language = %q, want en", got)
	}
}

// The size is a pop-up of the platform's sizes, not a number field with its
// own two buttons: the value shows on the button, and the menu below it chooses.
func TestTheFontSizePopupChoosesASize(t *testing.T) {
	sw, store := newTestSettingsWindow(t)
	sw.pane = paneTerminal
	tt := ui.NewTester(sw.view, 760, 600)
	tt.Frame()

	if err := tt.Click(formatSize(settings.DefaultFontSize) + " pt"); err != nil {
		t.Fatalf("open the size menu: %v", err)
	}
	tt.Frame()
	if !tt.HasText("16 pt") {
		t.Fatalf("the size menu has no 16 pt: texts = %v", tt.Texts())
	}
	if err := tt.Click("16 pt"); err != nil {
		t.Fatalf("choose 16 pt: %v", err)
	}
	tt.Frame()
	if sw.size != 16 {
		t.Errorf("size = %v, want 16", sw.size)
	}
	if got := store.Get().FontSize; got != 16 {
		t.Errorf("stored FontSize = %v, want 16", got)
	}
}

// The remote pane's fields have to be real text inputs, not in-place editable
// labels: clicking them and typing must reach the draft. This guards a
// regression where they were drawn with EditableText, so the pane showed a
// label with no input box under it.
func TestSettingsRemoteFieldsAcceptTyping(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	tt := ui.NewTester(sw.view, 640, 640)
	tt.Frame()
	if err := tt.Click(locales.SettingsPaneRelay()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsPaneRelay(), err)
	}
	tt.Frame()

	cases := []struct {
		label string
		typed string
		draft func() string
	}{
		{locales.SettingsRelayUrl(), "wss://relay.example/relay/agent", func() string { return sw.relay }},
		{locales.SettingsRelayDevice(), "mac-mini", func() string { return sw.device }},
		{locales.SettingsRelaySecret(), "s3cret", func() string { return sw.secret }},
	}
	for _, tc := range cases {
		if _, ok := tt.Find(tc.label); !ok {
			t.Fatalf("Find(%q): the field label is missing from the pane", tc.label)
		}
		// The label names its control in the form, so a click on it is a click
		// on the field: the same click a user makes, whichever side of the
		// label the control sits on.
		if err := tt.Click(tc.label); err != nil {
			t.Fatalf("Click(%q): %v", tc.label, err)
		}
		tt.Type(tc.typed)
		tt.Frame()
		if got := tc.draft(); got != tc.typed {
			t.Errorf("%s: typed %q but the draft holds %q", tc.label, tc.typed, got)
		}
	}
}

// 应用 with a relay is what makes the 机器 panel show anything but this machine.
// Nothing is typed: this machine proves itself with its own device credential,
// the relay hands it a short-lived credential to drive the fleet with, and that
// link is what carries the roster. Before this existed, the panel's own line
// told people to fill in a control that was not on the form.
func TestSettingsApplyingTheRelayReachesTheFleet(t *testing.T) {
	// A relay with this machine and another enrolled.
	reg, err := relay.OpenRegistry(filepath.Join(t.TempDir(), "devices.json"))
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	otherID, _, err := reg.Enroll("other box", "", "")
	if err != nil {
		t.Fatalf("enroll other box: %v", err)
	}
	thisID, thisSecret, err := reg.Enroll("this mac", "", "")
	if err != nil {
		t.Fatalf("enroll this mac: %v", err)
	}
	rl, err := relay.New(relay.Config{Registry: reg, ClientToken: "fleettoken"})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", rl.ServeAgent)
	mux.HandleFunc("/relay/client", rl.ServeClient)
	mux.HandleFunc("/relay/control-token", rl.ControlToken)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	f := newTestFleet(t)
	store := settings.Open(t.TempDir())
	w := newWindow(f, store, "", "")
	sw := newSettingsWindow(w, store, f)

	tt := ui.NewTester(sw.view, 640, 640)
	tt.Frame()
	if err := tt.Click(locales.SettingsPaneRelay()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsPaneRelay(), err)
	}
	tt.Frame()
	// There is no credential field to fill in: that is the point of the mint.
	// The relay hands this machine a short-lived credential of its own, so the
	// pane asks for this device's identity and says nothing about a token.
	for _, label := range []string{
		locales.SettingsRelayUrl(),
		locales.SettingsRelayDevice(),
		locales.SettingsRelaySecret(),
		locales.SettingsRelayCa(),
		locales.SettingsRelayOn(),
	} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("the relay pane is missing %q; texts = %v", label, tt.Texts())
		}
	}

	// Enrolled with a relay, with nothing else configured.
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/relay/agent"
	sw.relay, sw.device, sw.secret, sw.relayOn = url, thisID, thisSecret, true
	tt.Frame()
	if err := tt.Click(locales.SettingsApply()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsApply(), err)
	}
	tt.Frame()

	if !f.RelayOn() {
		t.Fatal("the member link was not asked for after applying the relay")
	}
	// The window's own client link, with a credential the relay minted.
	if f.RelayClient() == nil {
		t.Fatalf("no client link after applying: %s", f.RelayClientErr())
	}
	if f.ControlExpiry().IsZero() {
		t.Error("the link came up with no lifetime recorded for its credential")
	}
	if line := sw.reachStatusLine(); !strings.HasPrefix(line, locales.SettingsReachReady()) {
		t.Errorf("the pane says %q, want it to report the fleet is reachable", line)
	}

	// The roster follows, and the panel stops saying the token is missing.
	waitForRoster(t, w, otherID)
	if note := w.machineNote(); note != locales.MachineNoteReady() {
		t.Fatalf("the panel still asks for a token it no longer needs: %q", note)
	}

	// The other machine is not merely in the list: it is drawn as a tile in the
	// rail, and picking it is what shows its sessions in the column.
	nav := drawNav(t, w)
	var other tunnel.Machine
	for _, m := range w.roster {
		if m.ID == otherID {
			other = m
		}
	}
	label := w.machineRailLabel(other)
	if _, ok := nav.Find(label); !ok {
		t.Fatalf("the machines' rail does not hold the other machine; texts = %v", nav.Texts())
	}
	if err := nav.Click(label); err != nil {
		t.Fatal(err)
	}
	nav.Frame()
	if _, ok := nav.Find(w.machineDisplayName(otherID)); !ok {
		t.Fatalf("the machines' panel does not list the other machine; texts = %v", nav.Texts())
	}
}

// waitForRoster drains the window until the relay's roster names id. The roster
// is pushed to the window's own thread, so only a drain makes it visible here.
func waitForRoster(t *testing.T, w *window, id string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		w.drain()
		for _, m := range w.roster {
			if m.ID == id {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the roster never listed %s", id)
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
	if err := tt.Click(locales.SettingsPaneRelay()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsPaneRelay(), err)
	}
	tt.Frame()

	// Downloading a certificate from the admin page and then finding its path is
	// the friction these buttons exist to remove, so both must be visible
	// without filling anything in first.
	for _, want := range []string{locales.SettingsCaFetch(), locales.SettingsCaPick()} {
		if _, ok := tt.Find(want); !ok {
			t.Errorf("%q button is missing from the remote pane", want)
		}
	}
	if _, ok := tt.Find(locales.SettingsCaClear()); ok {
		t.Error("the clear button should only appear once a certificate is set")
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
	if err := tt.Click(locales.SettingsPaneRelay()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsPaneRelay(), err)
	}
	tt.Frame()
	// Find needs the whole element text, so assert on the note verbatim: the
	// property under test is that what the window holds reaches the screen. The
	// Contains check above already pins down what the note says.
	if _, ok := tt.Find(sw.caNote); !ok {
		t.Errorf("the certificate note is not rendered in the remote pane; texts = %v", tt.Texts())
	}
	if _, ok := tt.Find(locales.SettingsCaClear()); !ok {
		t.Error("the clear button should appear once a certificate is set")
	}
}

func TestSettingsCANoteExplainsAnUnusableFile(t *testing.T) {
	cases := []struct {
		name string
		path func(t *testing.T) string
		want func(t *testing.T, path string) string
	}{
		{
			name: "missing",
			path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "nope.pem") },
			want: func(t *testing.T, path string) string {
				_, err := os.ReadFile(path)
				if err == nil {
					t.Fatal("the file the note is about is readable after all")
				}
				return locales.SettingsCaUnreadable(err.Error())
			},
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
			want: func(t *testing.T, path string) string {
				pemBytes, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				_, err = agent.DescribeCert(pemBytes)
				if err == nil {
					t.Fatal("the junk file parsed as a certificate")
				}
				return locales.SettingsCaInvalid(err.Error())
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sw, _ := newTestSettingsWindow(t)
			path := tc.path(t)
			sw.ca = path
			sw.commit()
			if want := tc.want(t, path); sw.caNote != want {
				t.Errorf("note = %q, want %q", sw.caNote, want)
			}
		})
	}
}

func TestSettingsFetchCAWithoutAddressGuidesInsteadOfFailingQuietly(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	sw.relay = ""
	sw.fetchCA()
	if sw.caNote != locales.SettingsCaNeedUrl() {
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

	if _, ok := tt.Find(locales.SettingsDirty()); ok {
		t.Fatal("the pending-changes marker is shown before anything was edited")
	}

	sw.relay = "wss://relay.example/relay/agent"
	tt.Frame()
	if _, ok := tt.Find(locales.SettingsDirty()); !ok {
		t.Fatalf("no pending-changes marker after editing; texts = %v", tt.Texts())
	}

	if err := tt.Click(locales.SettingsApply()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsApply(), err)
	}
	tt.Frame()
	if _, ok := tt.Find(locales.SettingsDirty()); ok {
		t.Fatalf("the pending-changes marker survived the apply; texts = %v", tt.Texts())
	}
}

// The status line is the only confirmation the click worked, so it has to name
// the time it happened and be distinguishable from a failure.
func TestSettingsStatusReportsWhenItSaved(t *testing.T) {
	sw, _ := newTestSettingsWindow(t)
	sw.commit()

	saved := locales.SettingsSavedAt(sw.savedAt.Format("15:04:05"))
	if !strings.HasPrefix(sw.status, saved) {
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
	if err := tt.Click(locales.SettingsPaneRelay()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsPaneRelay(), err)
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
	if choices[0] != systemFamily() {
		t.Errorf("choices start with %q, want %q first", choices[0], systemFamily())
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

	rect, ok := tt.Find(locales.SettingsFont())
	if !ok {
		t.Fatalf("the font field is missing; texts = %v", tt.Texts())
	}
	// Start from an empty field so the caret position cannot change what is
	// typed into it.
	sw.family = ""
	tt.Frame()
	// The label focuses its control, as every field in the form does.
	tt.ClickAt(rect.X+rect.W/2, rect.Y+rect.H/2)
	tt.Type("Maple Mono NF")
	tt.Frame()

	if sw.family != "Maple Mono NF" {
		t.Fatalf("typed family = %q, want it in the draft", sw.family)
	}
	if err := tt.Click(locales.SettingsApply()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsApply(), err)
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

// 允许被控制 is the machine's answer to "who may drive the shells on this
// machine". It has to reach the hub — the thing that arbitrates — rather than
// being a note the window keeps to itself, and it has to work while clients are
// already attached.
func TestSettingsControlSwitchReachesTheHub(t *testing.T) {
	sw, store := newTestSettingsWindow(t)
	f := sw.fleet

	if !f.Hub().ControlAllowed() {
		t.Fatal("a fresh machine refuses other clients; joining a relay is the consent")
	}

	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()
	if err := tt.Click(locales.SettingsPaneServer()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsPaneServer(), err)
	}
	tt.Frame()
	if _, ok := tt.Find(locales.SettingsControl()); !ok {
		t.Fatalf("the pane has no control switch; texts = %v", tt.Texts())
	}
	if !sw.control {
		t.Fatal("the switch does not read the saved settings")
	}

	// The click a person makes: the label names the control, so clicking it
	// moves the switch.
	if err := tt.Click(locales.SettingsControl()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsControl(), err)
	}
	tt.Frame()
	if sw.control {
		t.Fatal("the switch did not move")
	}
	if f.Hub().ControlAllowed() {
		t.Fatal("turning the switch off left the hub allowing other clients to control it")
	}
	if !store.Get().ControlOff {
		t.Fatal("turning the switch off did not reach the settings file")
	}
	// The pane says what that state means, rather than leaving the switch to be
	// interpreted.
	if !tt.HasText(locales.SettingsControlOffLine()) {
		t.Fatalf("the pane does not explain the off state; texts = %v", tt.Texts())
	}

	// And back on, because a machine that can say no must be able to say yes.
	if err := tt.Click(locales.SettingsControl()); err != nil {
		t.Fatalf("Click(%s) again: %v", locales.SettingsControl(), err)
	}
	tt.Frame()
	if !f.Hub().ControlAllowed() || store.Get().ControlOff {
		t.Fatal("turning the switch back on did not reach the hub and the store")
	}
}

func TestSettingsAppearancePaneControls(t *testing.T) {
	sw, store := newTestSettingsWindow(t)
	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()

	if err := tt.Click(locales.SettingsPaneAppearance()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsPaneAppearance(), err)
	}
	tt.Frame()

	if !tt.HasText(locales.SettingsBgOpacity()) || !tt.HasText(locales.SettingsBgBlur()) {
		t.Fatalf("the appearance pane is missing opacity/blur controls; texts = %v", tt.Texts())
	}

	if !tt.HasText(locales.BlurNone()) || !tt.HasText(locales.BlurSubtle()) || !tt.HasText(locales.BlurMedium()) || !tt.HasText(locales.BlurDeep()) {
		t.Fatalf("the appearance pane is missing blur choices; texts = %v", tt.Texts())
	}

	// The window is acrylic by default, with no wash to pick: the pane shows the
	// acrylic controls only, at the acrylic default opacity.
	if sw.opacity != 0.80 {
		t.Fatalf("expected window opacity to default to 0.80, got %v", sw.opacity)
	}
	if got := store.Get().Opacity(); got != 0.80 {
		t.Fatalf("expected store opacity to default to 0.80, got %v", got)
	}

	// Click Deep blur
	if err := tt.Click(locales.BlurDeep()); err != nil {
		t.Fatalf("Click(%s): %v", locales.BlurDeep(), err)
	}
	tt.Frame()
	if store.Get().Blur() != "deep" {
		t.Fatalf("expected store blur to be 'deep', got %q", store.Get().Blur())
	}
}
