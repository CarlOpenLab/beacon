package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDesktopUsesMonochromeThemeForLegacyPreferences(t *testing.T) {
	for _, mode := range []string{themeSystem, themeLight, themeDark, "unexpected"} {
		theme := themeFor(mode)
		if theme == nil || !theme.Dark {
			t.Fatalf("themeFor(%q) did not return the fixed dark appearance", mode)
		}
		if got := theme.Background; got != ui.Hex("#111111") {
			t.Errorf("themeFor(%q) background = %v", mode, got)
		}
	}
}

func TestHeaderNamesCurrentSessionAndSidebarControl(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	tt := ui.NewTester(func(c *ui.Context) { applyTheme(c, ""); w.header(c, nil) }, 700, 52)
	if !tt.HasText("one") || !tt.HasText("本机") {
		t.Fatalf("header is missing session context: %v", tt.Texts())
	}
	if _, ok := tt.Find("显示或收起侧栏"); !ok {
		t.Fatal("header does not expose the sidebar toggle")
	}
}

func TestSidebarKeepsSessionsMachinesAndSettingsTogether(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tt := ui.NewTester(func(c *ui.Context) { applyTheme(c, ""); w.sidebar(c, "") }, sidebarWidth, 520)
	for _, label := range []string{"term-fleet", "会话", "one", "机器", "设置"} {
		if !tt.HasText(label) {
			t.Errorf("sidebar is missing %q: %v", label, tt.Texts())
		}
	}
}

func TestEmptyStateInvitesTheFirstShell(t *testing.T) {
	w, _ := newTestShellWindow(t)
	tt := ui.NewTester(func(c *ui.Context) { applyTheme(c, ""); w.emptyState(c) }, 600, 320)
	if !tt.HasText("还没有打开的终端") {
		t.Fatalf("empty window has no guidance: %v", tt.Texts())
	}
	if !tt.HasText("新建终端") {
		t.Fatal("empty state does not offer to start a shell")
	}
}

func TestTerminalPaletteKeepsANSIColors(t *testing.T) {
	for _, dark := range []bool{false, true} {
		palette := terminalTheme(dark)
		if palette.Background != ui.Hex("#111111") || palette.Cursor != ui.Hex("#dedede") {
			t.Errorf("terminal palette lost its dark shell background: %#v", palette)
		}
		if palette.Palette[1] == palette.Palette[2] || palette.Palette[2] == palette.Palette[4] {
			t.Errorf("red, green and blue ANSI colors collapsed together: %#v", palette.Palette)
		}
		if palette.Background != terminalTheme(!dark).Background {
			t.Error("legacy appearance changes the terminal background")
		}
	}
}

func TestSettingsButtonOpensInlinePreferences(t *testing.T) {
	w, _ := newTestShellWindow(t, "one")
	w.fleet = newTestFleet(t)
	tt := ui.NewTester(func(c *ui.Context) { w.view(c) }, 1100, 720)
	if err := tt.Click("设置 ⌘,"); err != nil {
		t.Fatalf("click settings row: %v", err)
	}
	tt.Frame()
	if !w.settingsOpen {
		t.Fatal("settings click did not open the dialog")
	}
	for _, text := range []string{"设置", "远程", "字体", "字号"} {
		if !tt.HasText(text) {
			t.Errorf("settings dialog is missing %q: %v", text, tt.Texts())
		}
	}
	if err := tt.Click("远程"); err != nil {
		t.Fatalf("open relay settings: %v", err)
	}
	tt.Frame()
	if !tt.HasText("公网地址") {
		t.Fatal("remote settings did not switch")
	}
	if err := tt.Click("CA 证书 ▸"); err != nil {
		t.Fatalf("expand CA certificate settings: %v", err)
	}
	tt.Frame()
	if !tt.HasText("从服务器获取") || !tt.HasText("选择文件…") {
		t.Fatalf("CA settings remain collapsed: %v", tt.Texts())
	}
	if err := tt.Click("关闭"); err != nil {
		t.Fatalf("close settings: %v", err)
	}
	tt.Frame()
	if w.settingsOpen {
		t.Fatal("settings dialog remained open")
	}
}
