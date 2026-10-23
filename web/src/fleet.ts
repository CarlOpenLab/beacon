// One WebSocket for the whole app. Every session shares the connection; the
// server multiplexes by `sessionId`.

import {
  CT,
  ST,
  CAP,
  MAX_UPLOAD_BYTES,
  MAX_UPLOAD_CHUNK,
  PROTOCOL_VERSION,
  decodeData,
  encodeBytes,
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
  // dismiss says the tab goes with the process: the shell was closed from
  // another client's tab strip, so this one drops its tab instead of keeping it
  // to read the last lines. 结束 ends the shell without it, and the tab stays.
  exited?(sessionId: string, exitCode: number, dismiss?: boolean): void
  closed?(sessionId: string): void
  errored?(code: string, message: string, sessionId?: string): void
}

interface Intent {
  cursor: number
}

const MIN_BACKOFF = 300
const MAX_BACKOFF = 5000

/**
 * Input is buffered per session and coalesced before it hits the wire. One
 * buffer solves two problems:
 *
 *   - A link that is not open used to swallow every keystroke typed into the
 *     gap: `Link.send` returns false and the bytes were gone with it. That is
 *     what "I typed half a command during a reconnect and it vanished" looks
 *     like from the outside. Bytes now wait here and `open()` flushes them
 *     once the session is re-attached — after the attach frames, on the same
 *     socket, so the hub sees a watcher before its input.
 *   - Holding a key or committing IME text fires several `onData` events
 *     within milliseconds. One frame each is affordable on a LAN and wasteful
 *     on the relay path, where the payload is JSON inside base64 inside JSON.
 *     The flush runs on the next task, so a lone keystroke pays no perceptible
 *     delay while a burst travels as one frame.
 */
const INPUT_COALESCE_BYTES = 4096

/**
 * How much typed input may wait while the link is down, per session. The
 * prefix is kept and the tail dropped past the cap: a shell needs the front of
 * a line to stay coherent, and no human reaches 64 KB of typing inside one
 * reconnect — only a paste does.
 */
const INPUT_PENDING_MAX = 64 * 1024

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
  /** Input typed but not yet on the wire, per session. */
  private pending = new Map<string, string>()
  /**
   * The lease epoch each session is at, from its last role frame. Input,
   * resize and close carry it: a frame at a past epoch is one the hub drops,
   * because the holder that sent it has been replaced.
   */
  private epochs = new Map<string, number>()
  private flushTimer?: number
  /** What the daemon answered it understands; empty until its hello. */
  private caps: string[] = []
  /** Output decodes in order even though zlib decompression is asynchronous. */
  private tails = new Map<string, Promise<void>>()
  /** Uploads waiting for their file.saved, keyed by request id. */
  private uploads = new Map<
    string,
    { resolve: (path: string) => void; reject: (err: Error) => void; timer: number }
  >()
  private nextUpload = 0

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
    clearTimeout(this.timer)
    clearTimeout(this.flushTimer)
    this.flushTimer = undefined
    // A deliberate close is not a blip: nothing typed now is worth replaying
    // into a link the user asked to drop.
    this.pending.clear()
    this.failUploads('the link was closed')
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
        this.send({ t: CT.hello, v: PROTOCOL_VERSION, caps: this.wantedCaps() })
        // Resume before the user notices: re-attach everything with the cursor we
        // already consumed, so a reconnect replays only the gap.
        for (const [sessionId, intent] of this.intents) {
          this.send({ t: CT.attach, sessionId, cursor: intent.cursor })
        }
        // Attach first, input second: same socket, same order, so the hub
        // never sees keystrokes for a session it has not attached yet.
        this.flushInput()
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
    this.failUploads('the machine is not reachable')
    this.setState('offline', detail)
    const delay = this.backoff
    this.backoff = Math.min(this.backoff * 2, MAX_BACKOFF)
    // Jitter so a fleet of clients does not reconnect in lockstep.
    const wait = delay / 2 + Math.random() * (delay / 2)
    clearTimeout(this.timer)
    this.timer = window.setTimeout(() => this.open(), wait)
  }

  /** What this page can do, announced in the hello. Compression is only
   * promised when the browser can decode it. */
  private wantedCaps(): string[] {
    const caps: string[] = [CAP.file]
    if (typeof DecompressionStream === 'function') caps.push(CAP.deflate)
    return caps
  }

  /** Whether the daemon said it shares a capability. */
  private can(cap: string): boolean {
    return this.caps.includes(cap)
  }

  /**
   * Decode one compressed output frame, in order. Decompression is async while
   * the frames arrive in order; a chain per session keeps the terminal from
   * receiving a later frame's bytes before an earlier one's.
   */
  private queueInflate(
    sessionId: string,
    cursor: number,
    data: Uint8Array,
    resync: boolean,
  ): void {
    const prev = this.tails.get(sessionId) ?? Promise.resolve()
    const next = prev
      .then(async () => {
        const out = await inflate(data)
        this.handlers.output?.(sessionId, cursor, out, resync)
      })
      .catch(() => {
        /* A frame that cannot be decoded is dropped, not fatal: the next attach
           replays the screen. */
      })
    this.tails.set(sessionId, next)
  }

  /**
   * Upload one file to the machine a session runs on and resolve with the path
   * it landed at. The shell is then given that path, not the bytes.
   */
  async putFile(sessionId: string, name: string, data: Uint8Array): Promise<string> {
    if (data.length === 0) throw new Error('empty file')
    if (data.length > MAX_UPLOAD_BYTES) throw new Error('file is too large')
    if (!this.can(CAP.file)) throw new Error('this machine is too old to receive files')

    const reqId = `p${++this.nextUpload}`
    const answer = new Promise<string>((resolve, reject) => {
      const timer = window.setTimeout(() => {
        this.uploads.delete(reqId)
        reject(new Error('upload timed out'))
      }, 45000)
      this.uploads.set(reqId, { resolve, reject, timer })
    })

    const head = this.send({
      t: CT.filePut,
      reqId,
      sessionId,
      name,
      size: data.length,
    })
    if (!head) {
      this.failUpload(reqId, 'connection is not open')
      return answer
    }
    for (let off = 0; off < data.length; off += MAX_UPLOAD_CHUNK) {
      const piece = data.subarray(off, Math.min(off + MAX_UPLOAD_CHUNK, data.length))
      if (!this.send({ t: CT.fileChunk, reqId, sessionId, data: encodeBytes(piece) })) {
        this.failUpload(reqId, 'connection closed during the upload')
        break
      }
    }
    this.send({ t: CT.fileDone, reqId, sessionId })
    return answer
  }

  private failUpload(reqId: string, reason: string): void {
    const pending = this.uploads.get(reqId)
    if (!pending) return
    this.uploads.delete(reqId)
    clearTimeout(pending.timer)
    pending.reject(new Error(reason))
  }

  private failUploads(reason: string): void {
    for (const id of [...this.uploads.keys()]) this.failUpload(id, reason)
  }

  private dispatch(msg: Msg): void {
    switch (msg.t) {
      case ST.hello: {
        this.host = msg.host
        this.version = msg.v ?? 0
        this.caps = msg.caps ?? []
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
        const data = decodeData(msg.data)
        if (msg.z) {
          this.queueInflate(sessionId, cursor, data, msg.resync === true)
          break
        }
        this.handlers.output?.(sessionId, cursor, data, msg.resync === true)
        break
      }
      case ST.fileSaved: {
        const pending = msg.reqId ? this.uploads.get(msg.reqId) : undefined
        if (!pending || !msg.reqId) break
        this.uploads.delete(msg.reqId)
        clearTimeout(pending.timer)
        if (msg.code) pending.reject(new Error(msg.message || msg.code))
        else if (msg.path) pending.resolve(msg.path)
        else pending.reject(new Error('the machine sent no path'))
        break
      }
      case ST.role: {
        if (msg.sessionId && msg.role) {
          // The epoch in force is remembered per session: every input, resize
          // and close frame carries it, and a frame at a past epoch is one the
          // hub drops because its holder has been replaced.
          this.epochs.set(msg.sessionId, msg.epoch ?? 0)
          this.handlers.role?.(msg.sessionId, msg.role, msg.controller ?? '')
        }
        break
      }
      case ST.exit: {
        if (msg.sessionId) this.handlers.exited?.(msg.sessionId, msg.exitCode ?? 0, msg.dismiss === true)
        break
      }
      case ST.closed: {
        const sessionId = msg.sessionId
        if (!sessionId) break
        this.intents.delete(sessionId)
        this.pending.delete(sessionId)
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

  create(opts: { cols: number; rows: number; cwd?: string; shell?: string; title?: string; from?: string }): void {
    this.send({ t: CT.create, ...opts })
  }

  /**
   * Name a session. The hub keeps the title and broadcasts it, so every client
   * watching the shell reads the new name; an empty title clears it and the
   * shell goes back to being named after its program.
   */
  rename(sessionId: string, title: string): void {
    this.send({ t: CT.rename, sessionId, title })
  }

  attach(sessionId: string, cursor = 0): void {
    this.intents.set(sessionId, { cursor })
    this.send({ t: CT.attach, sessionId, cursor })
  }

  detach(sessionId: string): void {
    this.intents.delete(sessionId)
    this.pending.delete(sessionId)
    this.send({ t: CT.detach, sessionId })
  }

  /**
   * Type into a session. Bytes are never dropped on the floor: while the link
   * is down they accumulate here (capped at INPUT_PENDING_MAX, prefix kept)
   * and go out when `open()` fires after a reconnect.
   */
  input(sessionId: string, text: string): void {
    if (!text) return
    const buf = (this.pending.get(sessionId) ?? '') + text
    this.pending.set(
      sessionId,
      buf.length > INPUT_PENDING_MAX ? buf.slice(0, INPUT_PENDING_MAX) : buf,
    )
    if (buf.length >= INPUT_COALESCE_BYTES) {
      this.flushInput(sessionId)
      return
    }
    this.scheduleFlush()
  }

  /**
   * Arm the coalescing flush, and only while the link is up: a flush that
   * cannot send would spin, and bytes typed into a dead link are already
   * waiting for `open()`.
   */
  private scheduleFlush(): void {
    if (this.flushTimer !== undefined || !this.online) return
    this.flushTimer = window.setTimeout(() => {
      this.flushTimer = undefined
      this.flushInput()
    }, 0)
  }

  /**
   * Send pending input, one frame per session. A frame that does not go out
   * stays pending: the next `open()` retries it, in order.
   */
  private flushInput(sessionId?: string): void {
    const ids = sessionId === undefined ? [...this.pending.keys()] : [sessionId]
    for (const id of ids) {
      const buf = this.pending.get(id)
      if (!buf) continue
      if (!this.send({ t: CT.input, sessionId: id, data: encodeData(buf), epoch: this.epochs.get(id) ?? 0 })) continue
      this.pending.delete(id)
    }
  }

  resize(sessionId: string, cols: number, rows: number): void {
    this.send({ t: CT.resize, sessionId, cols, rows, epoch: this.epochs.get(sessionId) ?? 0 })
  }

  /**
   * Ask for the lease. Steal takes it from a holder whose lease has not
   * lapsed; without it the hub refuses the claim and names the holder, which
   * is what lets a client offer "take it anyway" rather than assuming.
   */
  claim(sessionId: string, steal = false): void {
    this.send({ t: CT.claim, sessionId, steal: steal || undefined })
  }

  release(sessionId: string): void {
    this.send({ t: CT.release, sessionId })
  }

  /**
   * End the session: kill the process on its machine, for every watcher.
   *
   * dismiss carries the tab with it: the exit then removes the tab on every
   * client instead of leaving each one a pane showing 已退出. 结束 sends the
   * same close with `keep`, which is the act that leaves the last output.
   *
   * Both flags travel: a hub from 0.6.6 reads dismiss, and a later one reads
   * keep, so either daemon gets the answer the person meant.
   *
   * The epoch is deliberately not sent. It is only a guard against a frame from
   * a holder who has since been replaced, and the hub checks `controller == c`
   * anyway — which is the check that matters here. Sending the epoch a client
   * happens to remember makes the close fail in exactly the case it is most
   * needed: right after a claim, before the role frame that moves it has landed.
   */
  closeSession(sessionId: string, dismiss = false): void {
    this.send({ t: CT.close, sessionId, dismiss, keep: !dismiss })
  }
}

/**
 * Inflate one zlib frame with the browser's own decoder. A browser that does
 * not have DecompressionStream never advertised `deflate`, so a compressed
 * frame cannot reach this function there.
 */
async function inflate(data: Uint8Array): Promise<Uint8Array> {
  if (typeof DecompressionStream !== 'function') return data
  const stream = new Blob([data as BlobPart]).stream().pipeThrough(new DecompressionStream('deflate'))
  return new Uint8Array(await new Response(stream).arrayBuffer())
}
