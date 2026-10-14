// Wire protocol, mirroring internal/protocol/protocol.go.
//
// Keep field names byte-identical with the Go struct tags: the daemon rejects
// frames it cannot decode, and a typo here shows up as a silent no-op.

export const PROTOCOL_VERSION = 1

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
} as const

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
} as const

export interface Host {
  hostname: string
  platform: string
  arch: string
}

export interface Summary {
  sessionId: string
  pid: number
  cwd: string
  title: string
  cols: number
  rows: number
  alive: boolean
  cursor: number
  controller?: string
  viewers?: string[]
  createdAt: string
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

  sessionId?: string
  session?: Summary
  sessions?: Summary[]

  cols?: number
  rows?: number
  cwd?: string
  shell?: string
  title?: string

  /** Byte offset into the session output stream. Absent means 0. */
  cursor?: number
  /** Base64 on the wire (Go marshals []byte that way). */
  data?: string
  truncated?: boolean
  resync?: boolean

  role?: Role
  controller?: string

  code?: string
  message?: string

  exitCode?: number
}

/** Decode the base64 `data` field into raw bytes for xterm. */
export function decodeData(data: string | undefined): Uint8Array {
  if (!data) return new Uint8Array(0)
  const bin = atob(data)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

/** Encode terminal input bytes for the `data` field. */
export function encodeData(text: string): string {
  const bytes = new TextEncoder().encode(text)
  let bin = ''
  const CHUNK = 0x8000
  for (let i = 0; i < bytes.length; i += CHUNK) {
    bin += String.fromCharCode(...bytes.subarray(i, i + CHUNK))
  }
  return btoa(bin)
}