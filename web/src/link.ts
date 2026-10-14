// How a `Fleet` reaches a machine's session authority.
//
// A `Fleet` is one machine's session client: it owns the hello handshake, the
// attach/resume intents and the control-role bookkeeping. It does not care
// whether those frames travel over a WebSocket straight to the daemon, or over
// a multiplexed stream through the relay. That is what `Connector` abstracts —
// the same trick `hub.Transport` plays on the Go side.

import type { Msg } from './protocol'

export interface LinkHandlers {
  /** The link is usable; send the handshake now. */
  open(): void
  /** One inner session frame arrived. */
  message(msg: Msg): void
  /** The link is gone. `reason` is shown to the user. */
  close(reason: string): void
}

/** One live path to one machine's session authority. */
export interface Link {
  send(msg: Msg): boolean
  close(): void
  readonly ready: boolean
}

export interface Connector {
  /** Human-readable target, shown in the sidebar. */
  readonly label: string
  /** Whether the underlying transport is usable right now. */
  readonly up: boolean
  /** Open a link. One Fleet holds one link at a time. */
  connect(handlers: LinkHandlers): Link
}

/**
 * A link straight to a daemon's `/ws`.
 *
 * This is the local-network path and the only path M1 had. It stays first-class:
 * the desktop shell and a browser on the same LAN never touch the relay.
 */
export class DirectConnector implements Connector {
  private ws?: WebSocket
  readonly label: string

  constructor(
    private url: string,
    label?: string,
  ) {
    this.label = label ?? url
  }

  get up(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }

  connect(handlers: LinkHandlers): Link {
    const ws = new WebSocket(this.url)
    ws.binaryType = 'arraybuffer'
    this.ws = ws

    ws.onopen = () => handlers.open()
    ws.onmessage = (ev) => {
      if (typeof ev.data !== 'string') return
      let msg: Msg
      try {
        msg = JSON.parse(ev.data) as Msg
      } catch {
        return
      }
      handlers.message(msg)
    }
    ws.onerror = () => {
      /* onclose carries the reason */
    }
    ws.onclose = (ev) => handlers.close(ev.reason || '连接断开')

    return {
      send(msg: Msg): boolean {
        if (ws.readyState !== WebSocket.OPEN) return false
        ws.send(JSON.stringify(msg))
        return true
      },
      close(): void {
        ws.close()
      },
      get ready(): boolean {
        return ws.readyState === WebSocket.OPEN
      },
    }
  }
}