import { Terminal, type ITerminalOptions, type ITheme } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'

// The terminal's palette is the card's, on the terminal's own terms: kero draws
// a light terminal on a white card and a dark one on a graphite card, and the
// ANSI sixteen are Apple's system colors deepened for whichever ground they sit
// on. The numbers are internal/uitokens' TermLight / TermDark, token for token,
// so the browser and the desktop shell print the same colors.
const LIGHT: ITheme = {
  background: '#ffffff',
  foreground: '#1d1d1f',
  cursor: '#1d1d1f',
  cursorAccent: '#ffffff',
  selectionBackground: 'rgba(59, 108, 246, 0.22)',
  black: '#1d1d1f',
  red: '#d3302f',
  green: '#22863a',
  yellow: '#b08800',
  blue: '#2f5bd8',
  magenta: '#a033c4',
  cyan: '#0f8199',
  white: '#8e8e93',
  brightBlack: '#5f5f66',
  brightRed: '#ef4444',
  brightGreen: '#2fa84f',
  brightYellow: '#d4a017',
  brightBlue: '#3b82f6',
  brightMagenta: '#c04ee0',
  brightCyan: '#14a3bf',
  brightWhite: '#b4b4bb',
}

const DARK: ITheme = {
  background: '#232328',
  foreground: '#e6e6ea',
  cursor: '#e6e6ea',
  cursorAccent: '#161618',
  selectionBackground: 'rgba(47, 65, 112, 0.9)',
  black: '#3a3a40',
  red: '#ff6b64',
  green: '#4fd17a',
  yellow: '#f2c14e',
  blue: '#6b9bff',
  magenta: '#d07cf0',
  cyan: '#4cc9e0',
  white: '#c9c9cf',
  brightBlack: '#6c6c78',
  brightRed: '#ff8a84',
  brightGreen: '#74e397',
  brightYellow: '#ffd479',
  brightBlue: '#8fb3ff',
  brightMagenta: '#e09cf7',
  brightCyan: '#7fdcec',
  brightWhite: '#f5f5f7',
}

/** The palette for the appearance <html data-theme> is wearing right now. */
export function terminalTheme(): ITheme {
  return document.documentElement.dataset.theme === 'light' ? LIGHT : DARK
}

/** Repaint a terminal after the theme flips. */
export function applyTerminalTheme(term: Terminal): void {
  term.options.theme = terminalTheme()
}

const BASE: ITerminalOptions = {
  fontFamily:
    '"SF Mono", "JetBrains Mono", "Cascadia Mono", Menlo, Consolas, "Liberation Mono", monospace',
  fontSize: 12.5,
  lineHeight: 1.4,
  letterSpacing: 0,
  cursorBlink: true,
  cursorStyle: 'bar',
  scrollback: 10000,
  allowProposedApi: true,
  macOptionIsMeta: true,
  // The terminal is a display surface: input arrives through the composer bar
  // in the app layer, never through xterm's hidden textarea. Typing straight
  // into the network is what made input stutter on a slow link — every
  // keystroke paid a round trip, and every keystroke typed into a reconnect
  // gap used to vanish. Selection and copy still work with stdin off.
  disableStdin: true,
}

export interface TerminalBundle {
  term: Terminal
  fit: FitAddon
  dispose(): void
}

/**
 * A terminal plus its fit addon. Read-only-ness is structural: `disableStdin`
 * is on for every instance, so a viewer and a controller get the same
 * keystroke-free surface and only the app layer's composer decides what may
 * be sent.
 */
export function createTerminal(): TerminalBundle {
  const term = new Terminal({ ...BASE, theme: terminalTheme() })
  const fit = new FitAddon()
  term.loadAddon(fit)
  term.loadAddon(new WebLinksAddon())
  return {
    term,
    fit,
    dispose() {
      term.dispose()
    },
  }
}
