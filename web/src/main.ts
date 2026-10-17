import { App } from './app'
import { defaultDaemonURL } from './fleet'
import './style.css'
import './fleet.css'

/**
 * The same bundle is served by a daemon and by a relay, and those two want
 * different transports. Rather than build twice, ask the server which one it is.
 */
interface ServerConfig {
  mode: 'direct' | 'relay'
  ws: string
}

const TOKEN_KEY = 'termfleet.relay.token'
const URL_KEY = 'termfleet.relay.url'
const THEME_KEY = 'termfleet.theme'

/**
 * The palette lives on <html data-theme>, so both stylesheets key off one
 * attribute and a switch is a single flip. Applied before the app mounts to
 * avoid a flash of the wrong theme; the browser client is the only place this
 * runs, since the desktop shell is a native window.
 */
function applyTheme(): void {
  document.documentElement.dataset.theme = stored(THEME_KEY) === 'light' ? 'light' : 'dark'
}

/** Flip theme and remember it. Called from the status bar's control. */
function toggleTheme(): void {
  const next = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light'
  document.documentElement.dataset.theme = next
  store(THEME_KEY, next)
}

async function fetchConfig(): Promise<ServerConfig> {
  try {
    const res = await fetch('/tf/config', { headers: { accept: 'application/json' } })
    if (res.ok) {
      const cfg = (await res.json()) as ServerConfig
      if (cfg.mode === 'relay' || cfg.mode === 'direct') {
        return { mode: cfg.mode, ws: cfg.ws || (cfg.mode === 'direct' ? '/ws' : '/relay/client') }
      }
    }
  } catch {
    /* served from somewhere without /tf/config; fall back to direct */
  }
  return { mode: 'direct', ws: '/ws' }
}

function absoluteWS(path: string): string {
  if (/^wss?:\/\//i.test(path)) return path
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${location.host}${path.startsWith('/') ? path : `/${path}`}`
}

function stored(key: string): string {
  try {
    return localStorage.getItem(key) ?? ''
  } catch {
    return ''
  }
}

function store(key: string, value: string): void {
  try {
    localStorage.setItem(key, value)
  } catch {
    /* private mode; the value just will not be remembered */
  }
}

async function boot(): Promise<void> {
  const container = document.getElementById('app')
  if (!container) return

  applyTheme()

  const cfg = await fetchConfig()

  if (cfg.mode === 'direct') {
    // `?host=192.168.1.20:7678` still points one client at another machine on
    // the LAN; without it the page talks to whatever daemon served it.
    const explicit = new URLSearchParams(location.search).get('host')
    const ws = explicit ? defaultDaemonURL() : absoluteWS(cfg.ws)
    new App(container, { mode: 'direct', ws, onToggleTheme: toggleTheme }).start()
    return
  }

  // Relay mode. Prefer an address the user already used, then one the
  // deployment suggests, then whatever host is serving this page. `?relay=`
  // only steers development, where the relay is not the page's own host.
  let url = stored(URL_KEY) || absoluteWS(cfg.ws)
  const override = new URLSearchParams(location.search).get('relay')
  if (override) url = absoluteWS(override)

  const app = new App(container, {
    mode: 'relay',
    ws: url,
    token: stored(TOKEN_KEY),
    onToken: (token) => {
      store(TOKEN_KEY, token)
      store(URL_KEY, url)
    },
    onToggleTheme: toggleTheme,
  })
  app.start()
}

void boot()