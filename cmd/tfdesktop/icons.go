package main

import "github.com/egoist/mygo/ui"

// The icons are Lucide's drawings. Each one is an element on the 24-unit grid
// Lucide draws on, stroked in whatever colour the element it sits in carries, so
// one definition serves the light shell and the dark one.

// lucide wraps the shapes of an icon in the document they were drawn in.
func lucide(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" ` +
			`stroke="currentColor" stroke-width="1.9" stroke-linecap="round" ` +
			`stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	// iconTerminal marks a shell: the prompt, and the line being typed on.
	iconTerminal = lucide(`<path d="m4 17 6-6-6-6"/><path d="M12 19h8"/>`)
	// iconSun and iconMoon are the two sides of the appearance switch.
	iconSun = lucide(`<circle cx="12" cy="12" r="4"/><path d="M12 2v2"/>` +
		`<path d="M12 20v2"/><path d="m4.9 4.9 1.4 1.4"/><path d="m17.7 17.7 1.4 1.4"/>` +
		`<path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.3 17.7-1.4 1.4"/>` +
		`<path d="m19.1 4.9-1.4 1.4"/>`)
	iconMoon = lucide(`<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/>`)
	// iconSettings opens the settings window.
	iconSettings = lucide(`<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2Z"/><circle cx="12" cy="12" r="3"/>`)
	// iconPlus opens a shell, iconClose ends the one on screen.
	iconPlus    = lucide(`<path d="M5 12h14"/><path d="M12 5v14"/>`)
	iconClose   = lucide(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	iconRefresh = lucide(`<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>`)
	// iconInfo opens the session inspector: what the shell on screen is, beyond
	// what its own output says.
	iconInfo = lucide(`<circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/>`)
	// iconMonitor stands for another machine, which a shell can be taken from.
	iconMonitor = lucide(`<rect width="20" height="14" x="2" y="3" rx="2"/>` +
		`<path d="M8 21h8"/><path d="M12 17v4"/>`)
	// iconSend is the composer's send button: the command leaving this window.
	iconSend = lucide(`<path d="M14.5 21.7a.5.5 0 0 0 .9 0l6.5-19a.5.5 0 0 0-.6-.6l-19 6.5a.5.5 0 0 0 0 .9l7.9 3.2a2 2 0 0 1 1.1 1.1z"/>` +
		`<path d="m21.9 2.1-10.9 10.9"/>`)
	// iconSidebar hides and shows the column, as every Mac app's sidebar button
	// does: a window with its left third marked off.
	iconSidebar = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/>`)
	// iconChevron turns a group's arrow, and marks a row that opens something.
	iconChevron = lucide(`<path d="m9 18 6-6-6-6"/>`)
	// iconUpDown marks a pop-up button: the two ways its menu opens.
	iconUpDown = lucide(`<path d="m7 15 5 5 5-5"/><path d="m7 9 5-5 5 5"/>`)
	// iconSearch opens the command palette.
	iconSearch = lucide(`<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>`)
	// iconSplitRight and iconSplitDown split the pane on screen, as their
	// drawings do: one window, divided one way or the other.
	iconSplitRight = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M12 3v18"/>`)
	iconSplitDown  = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M3 12h18"/>`)
	// iconZoom fills the window with one pane; iconUnzoom puts the others back.
	iconZoom   = lucide(`<path d="M8 3H5a2 2 0 0 0-2 2v3"/><path d="M21 8V5a2 2 0 0 0-2-2h-3"/><path d="M3 16v3a2 2 0 0 0 2 2h3"/><path d="M16 21h3a2 2 0 0 0 2-2v-3"/>`)
	iconUnzoom = lucide(`<path d="M8 3v3a2 2 0 0 1-2 2H3"/><path d="M21 8h-3a2 2 0 0 1-2-2V3"/><path d="M3 16h3a2 2 0 0 1 2 2v3"/><path d="M16 21v-3a2 2 0 0 1 2-2h3"/>`)
	// iconKeyboard is the settings page of shortcuts.
	iconKeyboard = lucide(`<rect width="20" height="16" x="2" y="4" rx="2"/><path d="M6 8h.01"/><path d="M10 8h.01"/><path d="M14 8h.01"/><path d="M18 8h.01"/><path d="M8 12h8"/><path d="M6 12h.01"/><path d="M18 12h.01"/>`)
	// iconBell is a notification: the events the window tells the user about
	// while they are looking elsewhere.
	iconBell = lucide(`<path d="M10.268 21a2 2 0 0 0 3.464 0"/><path d="M3.262 15.326A1 1 0 0 0 4 17h16a1 1 0 0 0 .74-1.673C19.41 13.956 18 12.499 18 8A6 6 0 0 0 6 8c0 4.499-1.411 5.956-2.738 7.326"/>`)
	// iconCheck marks a chosen thing in a list of choices.
	iconCheck = lucide(`<path d="M20 6 9 17l-5-5"/>`)
	// iconFolder opens a project, or stands for the machine a row belongs to.
	iconFolder = lucide(`<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>`)
)
