package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

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
