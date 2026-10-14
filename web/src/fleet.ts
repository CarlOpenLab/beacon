// One WebSocket for the whole app. Every session shares the connection; the
// server multiplexes by `sessionId`.

import {
  CT,
  ST,
  PROTOCOL_VERSION,
  decodeData,
  encodeData,
  type Host,
  type Msg,
  type Role,
  type Summary,
} from './protocol'
import type { Connector, Link, LinkHandlers } from './link'

export type ConnState = 'connecting' | 'online' | 'offline'

export interface FleetHandlers {
  state?(state: ConnState, detail?: string): void
  hello?(host: Host, version: number): void
  sessions?(list: Summary[]): void
  created?(sessionId: string, session?: Summary): void
  attached?(sessionId: string, session: Summary | undefined, cursor: number, truncated: boolean): void
  output?(sessionId: string, cursor: number, data: Uint8Array, resync: boolean): void
  role?(sessionId: string, role: Role, controller: string): void
  exited?(sessionId: string, exitCode: number): void
  closed?(sessionId: string): void
  errored?(code: string, message: string, sessionId?: string): void
}

interface Intent {
  cursor: number
}

const MIN_BACKOFF = 300
const MAX_BACKOFF = 5000

/**
 * Derive the daemon URL.
 *
 * Served from the daemon (PC shell, phone browser) the page is same-origin, so
 * we just take the current host. Over `file://`, or when pointing one client at
 * another machine, `?host=192.168.1.20:7678` overrides it.
 */
export function defaultDaemonURL(): string {
  const override = new URLSearchParams(location.search).get('host')
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  if (override) {
    const withScheme = /^wss?:\/\//.test(override) ? override : `${proto}//${override}`
    return withScheme.replace(/\/+$/, '') + '/ws'
  }
  if (location.protocol === 'file:') return 'ws://127.0.0.1:7678/ws'
  return `${proto}//${location.host}/ws`
}

export class Fleet {
  private link?: Link
  private connector: Connector
  private backoff = MIN_BACKOFF
  private timer?: number
  private closedByUs = false
  /** Sessions this client wants attached, with the cursor it has consumed. */
  private intents = new Map<string, Intent>()

  /** Set by the app layer when this Fleet belongs to a machine in a roster. */
  machineId = ''

  state: ConnState = 'offline'
  host?: Host
  version = 0
  sessions: Summary[] = []

  constructor(
    private handlers: FleetHandlers = {},
    connector: Connector,
  ) {
    this.connector = connector
  }

  get endpoint(): string {
    return this.connector.label
  }

  connect(): void {
    this.closedByUs = false
    this.open()
  }

  /**
   * Connect only if there is no link yet.
   *
   * A relay hands out machines as its roster changes, so a naive `connect()`
   * per roster push would open a fresh socket (or stream) every time. This is
   * the idempotent form the app layer wants.
   */
  ensureConnected(): void {
    if (this.link || this.closedByUs) return
    this.open()
  }

  close(): void {
    this.closedByUs = true
    if (this.timer !== undefined) clearTimeout(this.timer)
    this.link?.close()
    this.link = undefined
    this.setState('offline', '已断开')
  }

  private setState(state: ConnState, detail?: string): void {
    this.state = state
    this.handlers.state?.(state, detail)
  }

  private open(): void {
    this.setState('connecting')
    const link = this.connector.connect({
      open: () => {
        this.backoff = MIN_BACKOFF
        this.setState('online')
        this.send({ t: CT.hello, v: PROTOCOL_VERSION })
        // Resume before the user notices: re-attach everything with the cursor we
        // already consumed, so a reconnect replays only the gap.
        for (const [sessionId, intent] of this.intents) {
          this.send({ t: CT.attach, sessionId, cursor: intent.cursor })
        }
      },
      message: (msg) => this.dispatch(msg),
      close: (reason) => {
        if (this.closedByUs) return
        this.retry(reason)
      },
    } satisfies LinkHandlers)
    this.link = link
  }

  private retry(detail: string): void {
    this.setState('offline', detail)
    const delay = this.backoff
    this.backoff = Math.min(this.backoff * 2, MAX_BACKOFF)
    // Jitter so a fleet of clients does not reconnect in lockstep.
    const wait = delay / 2 + Math.random() * (delay / 2)
    if (this.timer !== undefined) clearTimeout(this.timer)
    this.timer = window.setTimeout(() => this.open(), wait)
  }

  private dispatch(msg: Msg): void {
    switch (msg.t) {
      case ST.hello: {
        this.host = msg.host
        this.version = msg.v ?? 0
        this.sessions = msg.sessions ?? []
        this.handlers.hello?.(this.host ?? { hostname: '?', platform: '?', arch: '?' }, this.version)
        this.handlers.sessions?.(this.sessions)
        break
      }
      case ST.sessions: {
        this.sessions = msg.sessions ?? []
        this.handlers.sessions?.(this.sessions)
        break
      }
      case ST.created: {
        if (msg.sessionId) this.handlers.created?.(msg.sessionId, msg.session)
        break
      }
      case ST.attached: {
        const sessionId = msg.sessionId
        if (!sessionId) break
        const intent = this.intents.get(sessionId) ?? { cursor: 0 }
        intent.cursor = msg.cursor ?? 0
        this.intents.set(sessionId, intent)
        this.handlers.attached?.(sessionId, msg.session, msg.cursor ?? 0, msg.truncated === true)
        break
      }
      case ST.output: {
        const sessionId = msg.sessionId
        if (!sessionId) break
        const intent = this.intents.get(sessionId)
        const cursor = msg.cursor ?? 0
        if (intent) intent.cursor = cursor
        this.handlers.output?.(sessionId, cursor, decodeData(msg.data), msg.resync === true)
        break
      }
      case ST.role: {
        if (msg.sessionId && msg.role) {
          this.handlers.role?.(msg.sessionId, msg.role, msg.controller ?? '')
        }
        break
      }
      case ST.exit: {
        if (msg.sessionId) this.handlers.exited?.(msg.sessionId, msg.exitCode ?? 0)
        break
      }
      case ST.closed: {
        const sessionId = msg.sessionId
        if (!sessionId) break
        this.intents.delete(sessionId)
        this.handlers.closed?.(sessionId)
        break
      }
      case ST.error: {
        this.handlers.errored?.(msg.code ?? 'error', msg.message ?? '', msg.sessionId)
        break
      }
      default:
        break
    }
  }

  private send(msg: Msg): boolean {
    return this.link?.send(msg) ?? false
  }

  /** True once the link is live, for either transport. */
  get online(): boolean {
    return this.link?.ready ?? false
  }

  list(): void {
    this.send({ t: CT.list })
  }

  create(opts: { cols: number; rows: number; cwd?: string; shell?: string; title?: string }): void {
    this.send({ t: CT.create, ...opts })
  }

  attach(sessionId: string, cursor = 0): void {
    this.intents.set(sessionId, { cursor })
    this.send({ t: CT.attach, sessionId, cursor })
  }

  detach(sessionId: string): void {
    this.intents.delete(sessionId)
    this.send({ t: CT.detach, sessionId })
  }

  input(sessionId: string, text: string): void {
    if (!text) return
    this.send({ t: CT.input, sessionId, data: encodeData(text) })
  }

  resize(sessionId: string, cols: number, rows: number): void {
    this.send({ t: CT.resize, sessionId, cols, rows })
  }

  claim(sessionId: string): void {
    this.send({ t: CT.claim, sessionId })
  }

  release(sessionId: string): void {
    this.send({ t: CT.release, sessionId })
  }

  closeSession(sessionId: string): void {
    this.send({ t: CT.close, sessionId })
  }

  /** Last cursor consumed for a session, for callers that need to hand it off. */
  cursorOf(sessionId: string): number {
    return this.intents.get(sessionId)?.cursor ?? 0
  }
}
