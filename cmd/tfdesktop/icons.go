package main

import (
	"strconv"

	"github.com/egoist/mygo/ui"
)

// The icons are Lucide's drawings. Each one is an element on the 24-unit grid
// Lucide draws on, stroked in whatever colour the element it sits in carries, so
// one definition serves the light shell and the dark one.

// lucide wraps the shapes of an icon in the document they were drawn in: the
// 24-point tier's weight, for an icon drawn at 24 points.
func lucide(shapes string) *ui.SVG { return lucideDoc(shapes, 1.9) }

// lucideAt wraps the shapes in a document whose stroke is chosen for the size
// the icon will be drawn at, so the rendered weight stays on its tier instead
// of thinning with the picture: 1.9 points at 24, 1.75 at 16, 1.6 at 12. The
// stroke in the SVG's own 24-unit space grows as the drawn size shrinks. One
// shape string therefore serves every tier without a second parsed variant.
func lucideAt(shapes string, drawn float32) *ui.SVG {
	// One line weight on the rail: 1.5 points as drawn, which in the 24-unit
	// space the shapes live in is 1.5 × 24 ÷ drawn.
	return lucideDoc(shapes, float32(1.5)*24/drawn)
}

// lucideDoc wraps the shapes in a document stroked at the given weight, in the
// shapes' own 24-unit space.
func lucideDoc(shapes string, stroke float32) *ui.SVG {
	return ui.MustParseSVG([]byte(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" ` +
			`stroke="currentColor" stroke-width="` +
			strconv.FormatFloat(float64(stroke), 'f', -1, 32) +
			`" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

// lucideRail parses a machine glyph at the size the rail draws it. The rail is
// the 24-point tier, inside a 26-point tile.
func lucideRail(shapes string) *ui.SVG { return lucideAt(shapes, machineGlyphSize) }

// machineGlyphSVG is one machine glyph at the rail's weight, stroked or, for a
// machine that has failed, filled.
func machineGlyphSVG(key string, solid bool) *ui.SVG {
	shapes, ok := machineGlyphShapes[key]
	if !ok {
		return nil
	}
	doc := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="`
	if solid {
		doc += `currentColor`
	} else {
		doc += `none`
	}
	doc += `" stroke="currentColor" stroke-width="` +
		strconv.FormatFloat(float64(1.5*24/machineGlyphSize), 'f', -1, 32) +
		`" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`
	return ui.MustParseSVG([]byte(doc))
}

// The shape strings the rail and the rest of the window share.
const (
	terminalShape = `<path d="m4 17 6-6-6-6"/><path d="M12 19h8"/>`
	monitorShape  = `<rect width="20" height="14" x="2" y="3" rx="2"/>` +
		`<path d="M8 21h8"/><path d="M12 17v4"/>`
)

var (
	// iconTerminal marks a shell: the prompt, and the line being typed on.
	iconTerminal = lucide(terminalShape)
	// iconSun and iconMoon are the two sides of the appearance switch.
	iconSun = lucide(`<circle cx="12" cy="12" r="4"/><path d="M12 2v2"/>` +
		`<path d="M12 20v2"/><path d="m4.9 4.9 1.4 1.4"/><path d="m17.7 17.7 1.4 1.4"/>` +
		`<path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.3 17.7-1.4 1.4"/>` +
		`<path d="m19.1 4.9-1.4 1.4"/>`)
	iconMoon = lucide(`<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/>`)
	// iconSettings opens the settings window.
	iconSettings = lucideRail(`<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2Z"/><circle cx="12" cy="12" r="3"/>`)
	// iconPlus opens a shell, iconClose ends the one on screen.
	iconPlus    = lucide(`<path d="M5 12h14"/><path d="M12 5v14"/>`)
	iconClose   = lucide(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	iconRefresh = lucide(`<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>`)
	// iconInfo opens the session inspector: what the shell on screen is, beyond
	// what its own output says.
	iconInfo = lucide(`<circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/>`)
	// iconMonitor stands for another machine, which a shell can be taken from.
	iconMonitor = lucide(monitorShape)
	// The machine rail's glyphs, one per kind of machine a name can suggest.
	// They are all Lucide's drawings on the same 24-unit grid, so the tile can
	// switch between them without changing its weight. They are decoration: the
	// tile	// The machine rail's glyphs, by the key the settings file stores, on the
	// same 24-unit grid Lucide draws on. A machine that has failed is drawn
	// solid instead of stroked, so the difference between "this one is broken"
	// and the rest is a shape, not a colour a colour-blind reader may miss.
	machineGlyphShapes = map[string]string{
		"laptop": `<path d="M20 16V7a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v9"/>` +
			`<path d="M2 16h20"/>` +
			`<path d="M4 16v1a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-1"/>`,
		"server": `<rect width="20" height="8" x="2" y="2" rx="2"/>` +
			`<rect width="20" height="8" x="2" y="14" rx="2"/>` +
			`<path d="M6 6h.01"/><path d="M6 18h.01"/>`,
		"hard_drive": `<line x1="22" x2="2" y1="12" y2="12"/>` +
			`<path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/>` +
			`<line x1="6" x2="6.01" y1="16" y2="16"/>` +
			`<line x1="10" x2="10.01" y1="16" y2="16"/>`,
		"cpu": `<rect width="16" height="16" x="4" y="4" rx="2"/>` +
			`<rect width="6" height="6" x="9" y="9" rx="1"/>` +
			`<path d="M15 2v2"/><path d="M15 20v2"/>` +
			`<path d="M2 15h2"/><path d="M2 9h2"/>` +
			`<path d="M20 15h2"/><path d="M20 9h2"/>` +
			`<path d="M9 2v2"/><path d="M9 20v2"/>`,
		"database": `<ellipse cx="12" cy="5" rx="9" ry="3"/>` +
			`<path d="M3 5v14a9 3 0 0 0 18 0V5"/>` +
			`<path d="M3 12a9 3 0 0 0 18 0"/>`,
		"cloud": `<path d="M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z"/>`,
		"container": `<path d="M22 7.7c0-.6-.4-1.2-.8-1.4l-6.5-3.2c-.5-.3-1.2-.3-1.7 0L3.8 6.3c-.4.2-.8.8-.8 1.4v8.6c0 .6.4 1.2.8 1.4l6.5 3.2c.5.3 1.2.3 1.7 0l6.5-3.2c.4-.2.8-.8.8-1.4V7.7Z"/>` +
			`<path d="M10 10.5V22"/>` +
			`<path d="M2 7.5 12 12l10-4.5"/>` +
			`<path d="M7 19.5 12 16"/><path d="M17 19.5 12 16"/>`,
		"router": `<rect width="20" height="8" x="2" y="14" rx="2"/>` +
			`<path d="M6.01 18H6"/><path d="M10.01 18H10"/><path d="M15 18h1"/>` +
			`<path d="M17.84 7.17a4 4 0 0 0-5.66 0"/>` +
			`<path d="M20.66 4.34a8 8 0 0 0-11.31 0"/>`,
		"printer": `<path d="M6 18H4a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2h-2"/>` +
			`<path d="M6 9V3h12v6"/>` +
			`<rect x="6" y="14" width="12" height="8" rx="1"/>`,
		"box": `<path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/>` +
			`<path d="m3.3 7 8.7 5 8.7-5"/>` +
			`<path d="M12 22V12"/>`,
		"monitor":  monitorShape,
		"terminal": terminalShape,
	}

	iconLaptop       = machineGlyphSVG("laptop", false)
	iconServer       = machineGlyphSVG("server", false)
	iconHardDrive    = machineGlyphSVG("hard_drive", false)
	iconCPU          = machineGlyphSVG("cpu", false)
	iconDatabase     = machineGlyphSVG("database", false)
	iconCloud        = machineGlyphSVG("cloud", false)
	iconContainer    = machineGlyphSVG("container", false)
	iconRouter       = machineGlyphSVG("router", false)
	iconPrinter      = machineGlyphSVG("printer", false)
	iconBox          = machineGlyphSVG("box", false)
	iconMonitorRail  = machineGlyphSVG("monitor", false)
	iconTerminalRail = machineGlyphSVG("terminal", false) // iconSend is the composer's send button: the command leaving this window.
	iconSend         = lucide(`<path d="M14.5 21.7a.5.5 0 0 0 .9 0l6.5-19a.5.5 0 0 0-.6-.6l-19 6.5a.5.5 0 0 0 0 .9l7.9 3.2a2 2 0 0 1 1.1 1.1z"/>` +
		`<path d="m21.9 2.1-10.9 10.9"/>`)
	// iconSidebar hides and shows the column, as every Mac app's sidebar button
	// does: a window with its left third marked off.
	iconSidebar = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/>`)
	// iconChevron turns a group's arrow, and marks a row that opens something.
	iconChevron = lucide(`<path d="m9 18 6-6-6-6"/>`)
	// iconUpDown marks a pop-up button: the two ways its menu opens.
	iconUpDown = lucide(`<path d="m7 15 5 5 5-5"/><path d="m7 9 5-5 5 5"/>`)
	// iconSearch opens the command palette.
	iconSearch = lucideRail(`<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>`)
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
