// The relay's envelope, mirrored from internal/tunnel/frame.go.
//
// The relay never looks inside the payload: it routes bytes between one client
// stream and one member stream. Everything the session protocol says travels
// untouched, which is why adding session features never requires a relay
// release.

/** Tunnel frame types. */
export const TF = {
  auth: 'auth',
  authOK: 'auth.ok',
  machines: 'machines',
  open: 'open',
  accept: 'accept',
  data: 'data',
  close: 'close',
  error: 'error',
} as const

export interface TunnelMachine {
  id: string
  name: string
  online: boolean
  hostname?: string
  platform?: string
  arch?: string
  version?: number
}

export interface TunnelFrame {
  t: string
  /** Client-side stream id, allocated by the client. */
  stream?: number
  /** Target machine, only on `open`. */
  machine?: string
  /** Base64 of an inner session frame. */
  payload?: string
  machines?: TunnelMachine[]
  token?: string
  code?: string
  message?: string
}

/** The relay's own error codes we react to by name. */
export const TFERR = {
  unauthorized: 'unauthorized',
  machineOffline: 'machine_offline',
  streamLimit: 'stream_limit',
} as const

// ------------------------------------------------------------------ base64

/**
 * Base64 for the tunnel payload.
 *
 * The inner session frame is JSON containing base64 PTY bytes, so the payload
 * is JSON over JSON. It stays a string on the wire so a member can forward a
 * frame without decoding it.
 */
export function toBase64(text: string): string {
  const bytes = new TextEncoder().encode(text)
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return btoa(bin)
}

export function fromBase64(b64: string): string {
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return new TextDecoder().decode(bytes)
}