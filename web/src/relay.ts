// The client half of the relay protocol.
//
// One WebSocket carries every machine. Each machine the user looks at gets a
// logical *stream*, and inside that stream travels the identical session
// protocol the daemon has always spoken. The relay passes `payload` through
// without reading it, so nothing here knows what a "session" or a "cursor" is.

import type { Msg } from './protocol'
import type { Connector, Link, LinkHandlers } from './link'
import { fromBase64, TF, TFERR, toBase64, type TunnelFrame, type TunnelMachine } from './tunnel'

const MIN_BACKOFF = 500
const MAX_BACKOFF = 10_000

export type RelayState = 'auth' | 'online' | 'offline'

interface StreamEntry {
  machineId: string
  handlers: LinkHandlers
  /** Assigned by `open`; 0 while the machine is offline and we are waiting. */
  stream: number
  accepted: boolean
}

export interface RelayOptions {
  url: string
  token: string
  onState?: (state: RelayState, detail?: string) => void
  onMachines?: (machines: TunnelMachine[]) => void
}

export class RelayConnection {
  private ws?: WebSocket
  private authed = false
  private closedByUs = false
  private backoff = MIN_BACKOFF
  private timer?: number
  private nextStream = 1

  private byMachine = new Map<string, StreamEntry>()
  private byStream = new Map<number, StreamEntry>()

  machines: TunnelMachine[] = []
  state: RelayState = 'offline'

  constructor(private opts: RelayOptions) {}

  private setState(state: RelayState, detail?: string): void {
    this.state = state
    this.opts.onState?.(state, detail)
  }

  connect(): void {
    this.closedByUs = false
    this.open()
  }

  close(): void {
    this.closedByUs = true
    if (this.timer !== undefined) clearTimeout(this.timer)
    this.ws?.close()
    this.setState('offline', '已断开')
  }

  /** A Connector for one machine, held by that machine's `Fleet`. */
  connectorFor(machineId: string): Connector {
    const name = () => this.machines.find((m) => m.id === machineId)?.name ?? machineId
    const relay = this
    return {
      get label() {
        return name()
      },
      get up() {
        const m = relay.machines.find((x) => x.id === machineId)
        return relay.state === 'online' && !!m?.online
      },
      connect(handlers: LinkHandlers): Link {
        const entry = relay.register(machineId, handlers)
        return {
          send: (msg) => relay.sendOn(entry, msg),
          close: () => relay.unregister(entry),
          get ready() {
            return entry.accepted
          },
        }
      },
    }
  }

  // ------------------------------------------------------------ streams

  private register(machineId: string, handlers: LinkHandlers): StreamEntry {
    const existing = this.byMachine.get(machineId)
    if (existing) {
      // A Fleet retry: same slot, fresh handlers. The stream lives on if it is
      // already accepted, so a retry does not disturb a healthy session.
      existing.handlers = handlers
      if (existing.accepted) {
        // A microtask, not a direct call: the Fleet assigns its link only
        // after `connect()` returns, so frames sent from a synchronous
        // `open()` would have nowhere to go — the hello, the re-attach and any
        // input flushed into the gap would all be dropped on the floor.
        queueMicrotask(() => handlers.open())
      } else this.flush()
      return existing
    }
    const entry: StreamEntry = { machineId, handlers, stream: 0, accepted: false }
    this.byMachine.set(machineId, entry)
    this.flush()
    return entry
  }

  private unregister(entry: StreamEntry): void {
    this.byMachine.delete(entry.machineId)
    if (entry.stream) {
      this.byStream.delete(entry.stream)
      if (entry.accepted) this.raw({ t: TF.close, stream: entry.stream })
    }
    entry.accepted = false
  }

  /**
   * Open every stream whose machine is online, and close the ones whose machine
   * has gone away. Driven by the roster, so a machine coming back is picked up
   * without the client having to guess at retry timing.
   */
  private flush(): void {
    if (!this.authed) return
    for (const entry of this.byMachine.values()) {
      const online = this.machines.find((m) => m.id === entry.machineId)?.online ?? false
      if (online && !entry.stream) {
        entry.stream = this.nextStream++
        this.byStream.set(entry.stream, entry)
        this.raw({ t: TF.open, machine: entry.machineId, stream: entry.stream })
      } else if (!online && entry.stream) {
        // The machine dropped. Tear the stream down; the Fleet decides what to
        // tell the user, and re-registers when the roster brings it back.
        this.byStream.delete(entry.stream)
        entry.stream = 0
        entry.accepted = false
        entry.handlers.close('机器离线')
      }
    }
  }

  private sendOn(entry: StreamEntry, msg: Msg): boolean {
    if (!entry.accepted || !entry.stream) return false
    return this.raw({ t: TF.data, stream: entry.stream, payload: toBase64(JSON.stringify(msg)) })
  }

  private raw(frame: TunnelFrame): boolean {
    if (this.ws?.readyState !== WebSocket.OPEN) return false
    this.ws.send(JSON.stringify(frame))
    return true
  }

  // ------------------------------------------------------------ transport

  private open(): void {
    this.setState('auth')
    let ws: WebSocket
    try {
      ws = new WebSocket(this.opts.url)
    } catch (err) {
      this.retry(`无法连接中继：${err instanceof Error ? err.message : String(err)}`)
      return
    }
    ws.binaryType = 'arraybuffer'
    this.ws = ws

    ws.onopen = () => {
      // The token travels in the first frame, never in the URL: a query string
      // would land in access logs and browser history.
      this.raw({ t: TF.auth, token: this.opts.token })
    }
    ws.onmessage = (ev) => {
      if (typeof ev.data !== 'string') return
      let frame: TunnelFrame
      try {
        frame = JSON.parse(ev.data) as TunnelFrame
      } catch {
        return
      }
      this.handle(frame)
    }
    ws.onerror = () => {
      /* onclose carries the reason */
    }
    ws.onclose = (ev) => {
      this.authed = false
      for (const entry of this.byMachine.values()) {
        if (entry.accepted) {
          entry.accepted = false
          entry.handlers.close('中继连接断开')
        }
        entry.stream = 0
      }
      this.byStream.clear()
      if (this.closedByUs) return
      this.retry(ev.reason || '中继连接断开')
    }
  }

  private retry(detail: string): void {
    this.setState('offline', detail)
    const delay = this.backoff
    this.backoff = Math.min(this.backoff * 2, MAX_BACKOFF)
    const wait = delay / 2 + Math.random() * (delay / 2)
    if (this.timer !== undefined) clearTimeout(this.timer)
    this.timer = window.setTimeout(() => this.open(), wait)
  }

  private handle(frame: TunnelFrame): void {
    switch (frame.t) {
      case TF.authOK:
        this.backoff = MIN_BACKOFF
        this.authed = true
        this.setState('online')
        this.flush()
        return

      case TF.machines: {
        this.machines = frame.machines ?? []
        this.opts.onMachines?.(this.machines)
        this.flush()
        return
      }

      case TF.accept: {
        const entry = this.byStream.get(frame.stream ?? 0)
        if (!entry) return
        entry.accepted = true
        entry.handlers.open()
        return
      }

      case TF.data: {
        const entry = this.byStream.get(frame.stream ?? 0)
        if (!entry) return
        let msg: Msg
        try {
          msg = JSON.parse(fromBase64(frame.payload ?? '')) as Msg
        } catch {
          return
        }
        entry.handlers.message(msg)
        return
      }

      case TF.close: {
        const entry = this.byStream.get(frame.stream ?? 0)
        if (!entry) return
        this.byStream.delete(entry.stream)
        entry.stream = 0
        entry.accepted = false
        entry.handlers.close(frame.message || '会话已关闭')
        return
      }

      case TF.error: {
        if (frame.stream) {
          const entry = this.byStream.get(frame.stream)
          if (entry) {
            this.byStream.delete(entry.stream)
            entry.stream = 0
            entry.accepted = false
            entry.handlers.close(frame.message || '中继错误')
          }
          return
        }
        // Connection-level: a bad token is the one worth naming, because it is
        // the only one the user can fix.
        if (frame.code === TFERR.unauthorized) {
          this.closedByUs = true
          this.setState('offline', '访问令牌无效')
          this.ws?.close()
        }
        return
      }
    }
  }
}

/** Default relay endpoint for a page served by the relay itself. */
export function relayURLFromLocation(): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${location.host}/relay/client`
}