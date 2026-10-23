// Wire protocol, mirroring internal/protocol/protocol.go.
//
// Keep field names byte-identical with the Go struct tags: the daemon rejects
// frames it cannot decode, and a typo here shows up as a silent no-op.

export const PROTOCOL_VERSION = 2

/** Client -> server frame kinds. */
export const CT = {
  hello: 'hello',
  list: 'list',
  create: 'create',
  attach: 'attach',
  detach: 'detach',
  input: 'input',
  resize: 'resize',
  claim: 'claim',
  release: 'release',
  close: 'close',
  rename: 'rename',
  filePut: 'file.put',
  fileChunk: 'file.chunk',
  fileDone: 'file.done',
} as const

/** Server -> client frame kinds. */
export const ST = {
  hello: 'hello',
  sessions: 'sessions',
  created: 'created',
  attached: 'attached',
  closed: 'closed',
  exit: 'exit',
  output: 'output',
  role: 'role',
  error: 'error',
  fileSaved: 'file.saved',
} as const

/**
 * Capabilities a hello may announce. A feature that changes the wire is only
 * used when both ends named it, so an old daemon and a new page still talk.
 */
export const CAP = {
  deflate: 'deflate',
  file: 'file.put',
} as const

/** The largest file a paste may carry; the daemon enforces the same ceiling. */
export const MAX_UPLOAD_BYTES = 32 << 20
/** How much raw data one file.chunk frame carries. */
export const MAX_UPLOAD_CHUNK = 256 << 10

export const ROLE = {
  controller: 'controller',
  viewer: 'viewer',
} as const

export type Role = (typeof ROLE)[keyof typeof ROLE]

export const ERR = {
  protocol: 'protocol',
  badRequest: 'bad_request',
  noSession: 'no_session',
  notController: 'not_controller',
  badSize: 'bad_size',
  unauthorized: 'unauthorized',
  leaseHeld: 'lease_held',
  // The machine refuses other clients' control: watching is still allowed, and a
  // claim comes back with this rather than with a live lease to steal.
  controlOff: 'control_off',
} as const

export interface Host {
  hostname: string
  platform: string
  arch: string
  /** The build the daemon answers as; empty from one older than this field. */
  version?: string
  /** This running daemon's identity: hostname, pid and start time. */
  instance?: string
}

export interface Summary {
  sessionId: string
  pid: number
  cwd: string
  title: string
  /** The program the session runs; the name falls back to it until a title. */
  shell?: string
  cols: number
  rows: number
  alive: boolean
  cursor: number
  controller?: string
  viewers?: string[]
  createdAt: string
  /**
   * What the shell is doing, followed by the daemon from its output:
   * idle | working | waiting | error | exited. A daemon that does not report it
   * leaves it empty, and the client reads an empty one as idle.
   */
  status?: string
  /** A finish the person has not looked at yet. */
  unread?: boolean
  /** What the program said about its state, when it said anything. */
  message?: string
  /** What a waiting program wants: permission, question, auth. */
  kind?: string
  /** 0-100 while working, -1 when there is no progress to report. */
  progress?: number
  /** When the state began, in Unix milliseconds. */
  since?: number
  /**
   * The program in the foreground of the pane: "zsh" at a prompt, "claude"
   * while an agent runs. Empty on a host that cannot say.
   */
  process?: string
}

/**
 * One frame in either direction. Fields are optional because the daemon uses
 * `omitempty`: an absent `cursor` means 0, not "unspecified".
 */
export interface Msg {
  t: string
  v?: number
  reqId?: string
  host?: Host
  reason?: string
  /** Capabilities announced in a hello, in either direction. */
  caps?: string[]

  sessionId?: string
  session?: Summary
  sessions?: Summary[]

  cols?: number
  rows?: number
  cwd?: string
  shell?: string
  title?: string
  /**
   * Session whose working directory a create opens in: the directory that
   * shell stands in now, so a new tab continues where the one beside it is
   * rather than at the home directory. Absent means the daemon's own default.
   */
  from?: string

  /** Byte offset into the session output stream. Absent means 0. */
  cursor?: number
  /** Base64 on the wire (Go marshals []byte that way). */
  data?: string
  truncated?: boolean
  resync?: boolean
  /** `data` is zlib-compressed. Only sent between peers that share `deflate`. */
  z?: boolean

  /** file.put / file.chunk / file.done / file.saved. */
  name?: string
  size?: number
  /** Where the machine wrote an uploaded file: an absolute path. */
  path?: string

  role?: Role
  controller?: string

  /**
   * The lease. The role frame carries the epoch in force; input, resize, close
   * and claim carry the epoch the client believes it holds. A frame at a past
   * epoch belonged to a holder that has since been replaced and is dropped.
   */
  epoch?: number
  /** Take the lease from a live holder rather than asking for a free one. */
  steal?: boolean

  code?: string
  message?: string

  exitCode?: number
  /**
   * On an exit: a tab close rather than 结束 — the tab goes with the process on
   * every client, so this one removes its tab instead of keeping the last lines.
   */
  dismiss?: boolean
  /**
   * On a close: 结束, which ends the process and leaves the tab holding the
   * final output. Without it, a close takes the tab with it — the default a
   * client older than this field gets, since its two closes look the same.
   */
  keep?: boolean
}

/** Decode the base64 `data` field into raw bytes for xterm. */
export function decodeData(data: string | undefined): Uint8Array {
  if (!data) return new Uint8Array(0)
  const bin = atob(data)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

/** One encoder for the process: every keystroke used to allocate another. */
const encoder = new TextEncoder()

/** Encode terminal input bytes for the `data` field. */
export function encodeData(text: string): string {
  return encodeBytes(encoder.encode(text))
}

/** Encode raw bytes (an upload's chunk, a compressed frame) for `data`. */
export function encodeBytes(bytes: Uint8Array): string {
  let bin = ''
  const CHUNK = 0x8000
  for (let i = 0; i < bytes.length; i += CHUNK) {
    bin += String.fromCharCode(...bytes.subarray(i, i + CHUNK))
  }
  return btoa(bin)
}