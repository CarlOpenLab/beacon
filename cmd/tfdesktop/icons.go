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
)
