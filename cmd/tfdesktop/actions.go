package main

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/locales"
)

// The window's actions, in one list: what the menus, the command palette and the
// keyboard all name. An action has an id the settings file can name, a default
// accelerator, and something to do; the user's overrides live beside the
// defaults rather than replacing them, so Settings can always offer the default
// back.
//
// The accelerators are written the way this file parses them — "cmd+shift+d" —
// where "cmd" is the platform's shortcut modifier (Command on macOS, Control
// elsewhere), so a settings file carried between platforms keeps its meaning.
type action struct {
	id    string
	title func() string
	def   string
	run   func(w *window)
}

// appActions are the actions the window knows, in the order Settings lists them.
var appActions = []action{
	{"session.new", locales.ActionNewTerminal, "cmd+t", func(w *window) { w.newTab() }},
	{"session.rename", locales.TabRename, "cmd+r", func(w *window) {
		if tb := w.activeTab(); tb != nil {
			w.beginRename(tb.machine, tb.id, tb.name())
		}
	}},
	{"session.next", locales.ActionNextSession, "cmd+shift+bracketright", func(w *window) { w.stepTab(1) }},
	{"session.previous", locales.ActionPreviousSession, "cmd+shift+bracketleft", func(w *window) { w.stepTab(-1) }},
	{"session.attention", locales.ActionJumpAttention, "cmd+j", func(w *window) { w.jumpToAttention() }},
	{"sidebar.toggle", locales.ActionToggleSidebar, "cmd+b", func(w *window) { w.sidebarHidden = !w.sidebarHidden }},

	{"pane.split.right", locales.ActionSplitRight, "cmd+d", func(w *window) { w.splitFocused(true) }},
	{"pane.split.down", locales.ActionSplitDown, "cmd+shift+d", func(w *window) { w.splitFocused(false) }},
	{"pane.zoom", locales.ActionZoomPane, "cmd+shift+enter", func(w *window) { w.zoomFocused() }},
	{"pane.next", locales.ActionNextPane, "cmd+bracketright", func(w *window) { w.cyclePane(1) }},
	{"pane.previous", locales.ActionPreviousPane, "cmd+bracketleft", func(w *window) { w.cyclePane(-1) }},
	{"pane.left", locales.ActionPaneLeft, "cmd+alt+left", func(w *window) { w.focusDirection("left") }},
	{"pane.right", locales.ActionPaneRight, "cmd+alt+right", func(w *window) { w.focusDirection("right") }},
	{"pane.up", locales.ActionPaneUp, "cmd+alt+up", func(w *window) { w.focusDirection("up") }},
	{"pane.down", locales.ActionPaneDown, "cmd+alt+down", func(w *window) { w.focusDirection("down") }},

	{"pane.close", locales.TabCloseTab, "cmd+w", func(w *window) { w.closeSplitPane() }},
	{"session.refresh", locales.SessionRefresh, "f5", func(w *window) { w.refreshAll() }},

	{"palette.open", locales.ActionPalette, "cmd+p", func(w *window) { w.openPalette() }},
	{"settings.open", locales.ActionSettings, "cmd+comma", func(w *window) { w.openSettings() }},

	{"text.bigger", locales.ActionTextBigger, "cmd+equal", func(w *window) { w.zoomText(1) }},
	{"text.smaller", locales.ActionTextSmaller, "cmd+minus", func(w *window) { w.zoomText(-1) }},
	{"text.reset", locales.ActionTextReset, "cmd+0", func(w *window) { w.zoomText(0) }},
}

// actionByID finds an action.
func actionByID(id string) (action, bool) {
	for _, a := range appActions {
		if a.id == id {
			return a, true
		}
	}
	return action{}, false
}

// accel is an action's accelerator as the user last set it, or its default when
// they never did. An empty override is a deliberate unbinding, which is why the
// map is consulted rather than the value alone.
func (w *window) accel(a action) string {
	if w.store != nil {
		if keys, ok := w.store.Get().Keybinds[a.id]; ok {
			return keys
		}
	}
	return a.def
}

// keyFor is an action's accelerator as a button shows it, empty when the action
// is unknown or unbound.
func (w *window) keyFor(id string) string {
	if a, ok := actionByID(id); ok {
		return glyphAccel(w.accel(a))
	}
	return ""
}

// runShortcuts dispatches the keyboard: every action whose accelerator was just
// pressed, first one wins. It is the one place a shortcut is read, so rebinding
// one in Settings changes the window and not only the label there.
func (w *window) runShortcuts(c *ui.Context) {
	if w.settingsOpen || w.reachPrompt != "" || w.pal.open {
		return
	}
	for _, a := range appActions {
		accel := w.accel(a)
		if accel == "" {
			continue
		}
		mods, key, ok := parseAccel(accel)
		if !ok {
			continue
		}
		if c.Shortcut(mods, key) {
			a.run(w)
			return
		}
	}
}

// --- accelerators ---

// keyNames maps the names settings files use to the keys they mean. The names
// are the ones a Go program would use for the character: "comma" for "," and
// "bracketleft" for "[".
var keyNames = func() map[string]ui.Key {
	m := map[string]ui.Key{
		"enter": ui.KeyEnter, "return": ui.KeyEnter,
		"escape": ui.KeyEscape, "esc": ui.KeyEscape,
		"backspace": ui.KeyBackspace, "delete": ui.KeyDelete, "insert": ui.KeyInsert,
		"tab": ui.KeyTab, "space": ui.KeySpace,
		"home": ui.KeyHome, "end": ui.KeyEnd,
		"pageup": ui.KeyPageUp, "pagedown": ui.KeyPageDown,
		"left": ui.KeyLeft, "right": ui.KeyRight, "up": ui.KeyUp, "down": ui.KeyDown,
		"minus": ui.KeyMinus, "equal": ui.KeyEqual, "comma": ui.KeyComma,
		"period": ui.KeyPeriod, "slash": ui.KeySlash, "semicolon": ui.KeySemicolon,
		"quote": ui.KeyQuote, "bracketleft": ui.KeyBracketLeft,
		"bracketright": ui.KeyBracketRight, "backslash": ui.KeyBackslash,
		"backquote": ui.KeyBackquote, "grave": ui.KeyBackquote,
		",": ui.KeyComma, ".": ui.KeyPeriod, "/": ui.KeySlash, ";": ui.KeySemicolon,
		"'": ui.KeyQuote, "[": ui.KeyBracketLeft, "]": ui.KeyBracketRight,
		"\\": ui.KeyBackslash, "`": ui.KeyBackquote, "-": ui.KeyMinus, "=": ui.KeyEqual,
	}
	letters := []struct {
		name string
		key  ui.Key
	}{
		{"a", ui.KeyA}, {"b", ui.KeyB}, {"c", ui.KeyC}, {"d", ui.KeyD}, {"e", ui.KeyE},
		{"f", ui.KeyF}, {"g", ui.KeyG}, {"h", ui.KeyH}, {"i", ui.KeyI}, {"j", ui.KeyJ},
		{"k", ui.KeyK}, {"l", ui.KeyL}, {"m", ui.KeyM}, {"n", ui.KeyN}, {"o", ui.KeyO},
		{"p", ui.KeyP}, {"q", ui.KeyQ}, {"r", ui.KeyR}, {"s", ui.KeyS}, {"t", ui.KeyT},
		{"u", ui.KeyU}, {"v", ui.KeyV}, {"w", ui.KeyW}, {"x", ui.KeyX}, {"y", ui.KeyY},
		{"z", ui.KeyZ},
	}
	for _, l := range letters {
		m[l.name] = l.key
	}
	digits := []ui.Key{ui.Key0, ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9}
	for i, k := range digits {
		m[string(rune('0'+i))] = k
	}
	fkeys := []ui.Key{ui.KeyF1, ui.KeyF2, ui.KeyF3, ui.KeyF4, ui.KeyF5, ui.KeyF6, ui.KeyF7, ui.KeyF8, ui.KeyF9, ui.KeyF10, ui.KeyF11, ui.KeyF12}
	for i, k := range fkeys {
		m["f"+itoa(i+1)] = k
	}
	return m
}()

// keyName is the name an accelerator writes for a key: the inverse of keyNames,
// built from the canonical spellings alone so that "," and "comma" — which both
// parse — cannot make the written form depend on map order.
var keyName = func() map[ui.Key]string {
	m := make(map[ui.Key]string, 64)
	add := func(key ui.Key, name string) { m[key] = name }
	for i := 0; i < 26; i++ {
		add(ui.KeyA+ui.Key(i), string(rune('a'+i)))
	}
	for i := 0; i < 10; i++ {
		add(ui.Key0+ui.Key(i), string(rune('0'+i)))
	}
	for i := 0; i < 12; i++ {
		add(ui.KeyF1+ui.Key(i), "f"+itoa(i+1))
	}
	for name, key := range map[string]ui.Key{
		"enter": ui.KeyEnter, "escape": ui.KeyEscape, "backspace": ui.KeyBackspace,
		"delete": ui.KeyDelete, "insert": ui.KeyInsert, "tab": ui.KeyTab,
		"space": ui.KeySpace, "home": ui.KeyHome, "end": ui.KeyEnd,
		"pageup": ui.KeyPageUp, "pagedown": ui.KeyPageDown,
		"left": ui.KeyLeft, "right": ui.KeyRight, "up": ui.KeyUp, "down": ui.KeyDown,
		"minus": ui.KeyMinus, "equal": ui.KeyEqual, "comma": ui.KeyComma,
		"period": ui.KeyPeriod, "slash": ui.KeySlash, "semicolon": ui.KeySemicolon,
		"quote": ui.KeyQuote, "bracketleft": ui.KeyBracketLeft,
		"bracketright": ui.KeyBracketRight, "backslash": ui.KeyBackslash,
		"backquote": ui.KeyBackquote,
	} {
		add(key, name)
	}
	return m
}()

// parseAccel reads "cmd+shift+d" into the modifiers and the key.
func parseAccel(s string) (ui.Modifiers, ui.Key, bool) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), "+")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return 0, ui.KeyUnknown, false
	}
	var mods ui.Modifiers
	for _, p := range parts[:len(parts)-1] {
		switch p {
		case "cmd", "cmdorctrl", "command", "super":
			mods |= ui.Cmd
		case "ctrl", "control":
			mods |= ui.Ctrl
		case "shift":
			mods |= ui.Shift
		case "alt", "opt", "option":
			mods |= ui.Alt
		default:
			return 0, ui.KeyUnknown, false
		}
	}
	key, ok := keyNames[parts[len(parts)-1]]
	if !ok {
		return 0, ui.KeyUnknown, false
	}
	return mods, key, true
}

// formatAccel writes modifiers and a key back into the storage form, canonically:
// the modifiers in a fixed order, the key last.
func formatAccel(mods ui.Modifiers, key ui.Key) string {
	name, ok := keyName[key]
	if !ok {
		return ""
	}
	var parts []string
	// Cmd and Ctrl are one bit on Windows and Linux, where Control is the
	// platform's shortcut modifier: write the canonical name once, never both.
	switch {
	case mods&ui.Cmd != 0:
		parts = append(parts, "cmd")
	case mods&ui.Ctrl != 0:
		parts = append(parts, "ctrl")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "alt")
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "shift")
	}
	return strings.Join(append(parts, name), "+")
}

// glyphAccel writes an accelerator the way the platform's menus do: "⇧⌘D" on
// macOS, "Ctrl+Shift+D" elsewhere. An empty accelerator is "".
func glyphAccel(accel string) string {
	mods, key, ok := parseAccel(accel)
	if !ok {
		return accel
	}
	name, ok := keyName[key]
	if !ok {
		return accel
	}
	label := strings.ToUpper(name)
	switch name {
	case "comma":
		label = ","
	case "period":
		label = "."
	case "slash":
		label = "/"
	case "backslash":
		label = "\\"
	case "minus":
		label = "-"
	case "equal":
		label = "="
	case "semicolon":
		label = ";"
	case "quote":
		label = "'"
	case "bracketleft":
		label = "["
	case "bracketright":
		label = "]"
	case "backquote":
		label = "`"
	case "space":
		label = "Space"
	case "enter":
		label = "↩"
	case "tab":
		label = "⇥"
	case "escape":
		label = "Esc"
	case "backspace":
		label = "⌫"
	case "delete":
		label = "Del"
	case "left":
		label = "←"
	case "right":
		label = "→"
	case "up":
		label = "↑"
	case "down":
		label = "↓"
	}
	if runtime.GOOS != "darwin" {
		var parts []string
		if mods&ui.Ctrl != 0 || mods&ui.Cmd != 0 {
			parts = append(parts, "Ctrl")
		}
		if mods&ui.Alt != 0 {
			parts = append(parts, "Alt")
		}
		if mods&ui.Shift != 0 {
			parts = append(parts, "Shift")
		}
		return strings.Join(append(parts, label), "+")
	}
	glyph := ""
	if mods&ui.Ctrl != 0 && mods&ui.Cmd == 0 {
		glyph += "⌃"
	}
	if mods&ui.Alt != 0 {
		glyph += "⌥"
	}
	if mods&ui.Shift != 0 {
		glyph += "⇧"
	}
	if mods&ui.Cmd != 0 {
		glyph += "⌘"
	}
	return glyph + label
}

// menuAccel writes an accelerator in the form mygo's menus take:
// "CmdOrCtrl+Shift+D".
func menuAccel(accel string) string {
	mods, key, ok := parseAccel(accel)
	if !ok {
		return ""
	}
	name, ok := keyName[key]
	if !ok {
		return ""
	}
	label := strings.ToUpper(name)
	switch name {
	case "comma":
		label = ","
	case "period":
		label = "."
	case "slash":
		label = "/"
	case "minus":
		label = "-"
	case "equal":
		label = "="
	case "semicolon":
		label = ";"
	case "quote":
		label = "'"
	case "bracketleft":
		label = "["
	case "bracketright":
		label = "]"
	case "backquote":
		label = "`"
	case "space":
		label = "Space"
	case "enter":
		label = "Enter"
	case "tab":
		label = "Tab"
	case "escape":
		label = "Escape"
	case "backspace":
		label = "Backspace"
	case "delete":
		label = "Delete"
	case "left", "right", "up", "down":
		label = strings.ToUpper(name[:1]) + name[1:]
	}
	var parts []string
	// One bit on Windows and Linux: spell it the platform's way, once.
	switch {
	case mods&ui.Cmd != 0:
		parts = append(parts, "CmdOrCtrl")
	case mods&ui.Ctrl != 0:
		parts = append(parts, "Ctrl")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	return strings.Join(append(parts, label), "+")
}
