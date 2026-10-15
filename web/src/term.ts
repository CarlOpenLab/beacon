import { Terminal, type ITerminalOptions } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'

export const THEME = {
  background: '#0b0e14',
  foreground: '#c8ccd4',
  cursor: '#7aa2f7',
  cursorAccent: '#0b0e14',
  selectionBackground: '#2d3650',
  black: '#1c2128',
  red: '#f07178',
  green: '#7ec699',
  yellow: '#e6b673',
  blue: '#7aa2f7',
  magenta: '#bb9af7',
  cyan: '#7dcfff',
  white: '#c8ccd4',
  brightBlack: '#4a5160',
  brightRed: '#ff8b92',
  brightGreen: '#9ad8b1',
  brightYellow: '#f5cd8b',
  brightBlue: '#9ab8ff',
  brightMagenta: '#d0b3ff',
  brightCyan: '#a3e2ff',
  brightWhite: '#e8ebf0',
} as const

const BASE: ITerminalOptions = {
  fontFamily:
    '"SF Mono", "JetBrains Mono", "Cascadia Mono", Menlo, Consolas, "Liberation Mono", monospace',
  fontSize: 13,
  lineHeight: 1.25,
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