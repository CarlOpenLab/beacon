import { Terminal, type ITerminalOptions } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'

// The terminal's palette is the chrome's, on the terminal's own terms: the
// cursor and the ANSI magenta are the violet the accent is, so a prompt and a
// chosen tab are the same colour. The sheet is dark in both themes — the ANSI
// sixteen are drawn for a dark ground — which is why these are constants and
// not CSS variables on <html data-theme>. cmd/tfdesktop/palette.go's
// terminalTheme carries the same numbers for the desktop shell.
export const THEME = {
  background: '#0d0d12',
  foreground: '#d7dae2',
  cursor: '#b6a6ff',
  cursorAccent: '#0d0d12',
  selectionBackground: 'rgba(182, 166, 255, 0.28)',
  black: '#26262e',
  red: '#ff8b93',
  green: '#6fd39a',
  yellow: '#e6c07d',
  blue: '#9ecbff',
  magenta: '#b6a6ff',
  cyan: '#7dd3fc',
  white: '#d7dae2',
  brightBlack: '#4a4a56',
  brightRed: '#ffa3aa',
  brightGreen: '#8ce3ae',
  brightYellow: '#f0d095',
  brightBlue: '#b3d7ff',
  brightMagenta: '#c9bcff',
  brightCyan: '#9be0ff',
  brightWhite: '#eef0f6',
} as const

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
  theme: THEME,
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
  const term = new Terminal({ ...BASE })
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