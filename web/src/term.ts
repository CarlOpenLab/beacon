import { Terminal, type ITerminalOptions } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'

export const THEME = {
  background: '#0a0d12',
  foreground: '#d3dae3',
  cursor: '#3adfae',
  cursorAccent: '#0a0d12',
  selectionBackground: '#24313f',
  black: '#1b2430',
  red: '#f0777a',
  green: '#3adfae',
  yellow: '#e6bb66',
  blue: '#7aa7c9',
  magenta: '#c39ad6',
  cyan: '#6fc7d8',
  white: '#d3dae3',
  brightBlack: '#4b5766',
  brightRed: '#ff9a9c',
  brightGreen: '#5cebbb',
  brightYellow: '#f2cd85',
  brightBlue: '#93bede',
  brightMagenta: '#d5b2e6',
  brightCyan: '#8bd9e7',
  brightWhite: '#eef2f7',
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