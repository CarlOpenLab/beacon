// Acceptance for the relay chain, driven against the real binaries.
//
// The Go tests already prove the routing logic with a real daemon in-process.
// This script exists to catch the things that only break once everything is a
// separate process: flag wiring, the enroll → configure handshake, and the
// base64 layering a browser has to do.
//
//   pnpm run acceptance:relay
//
// Zero dependencies: Node 22+ has fetch and WebSocket built in.

import { spawn } from 'node:child_process'
import { mkdtempSync, rmSync, readFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const RELAY_PORT = 7698
const DAEMON_PORT = 7679
const RELAY = `127.0.0.1:${RELAY_PORT}`
const TOKEN = 'acceptance-client-token'
const MARKER = 'RELAY_CHAIN_OK'

let pass = 0
let fail = 0
const check = (name, ok, detail = '') => {
  if (ok) {
    pass++
    console.log(`  ok   ${name}`)
  } else {
    fail++
    console.log(`  FAIL ${name}${detail ? ` — ${detail}` : ''}`)
  }
}

const work = mkdtempSync(join(tmpdir(), 'tfrelay-'))
const devices = join(work, 'devices.json')
const procs = []

function launch(cmd, args, opts = {}) {
  const p = spawn(cmd, args, { stdio: ['ignore', 'pipe', 'pipe'], ...opts })
  p.stdout.on('data', () => {})
  p.stderr.on('data', (d) => {
    if (process.env.TF_VERBOSE) process.stderr.write(`[${cmd}] ${d}`)
  })
  procs.push(p)
  return p
}

function cleanup() {
  for (const p of procs) {
    try {
      p.kill('SIGKILL')
    } catch {}
  }
  try {
    rmSync(work, { recursive: true, force: true })
  } catch {}
}

async function waitFor(fn, what, timeoutMs = 15000) {
  const deadline = Date.now() + timeoutMs
  let last
  while (Date.now() < deadline) {
    try {
      const v = await fn()
      if (v) return v
    } catch (err) {
      last = err
    }
    await new Promise((r) => setTimeout(r, 150))
  }
  throw new Error(`timed out waiting for ${what}${last ? `: ${last.message}` : ''}`)
}

// A client mirroring what the browser will do: one socket, N streams, and the
// same double base64 (payload is bytes, and so is protocol.Msg.Data).
class Client {
  constructor(ws) {
    this.ws = ws
    this.inbox = []
    this.waiters = []
    ws.addEventListener('message', (ev) => {
      const f = JSON.parse(typeof ev.data === 'string' ? ev.data : ev.data.toString())
      const w = this.waiters.shift()
      if (w) w(f)
      else this.inbox.push(f)
    })
  }

  static async dial(token) {
    const ws = new WebSocket(`ws://${RELAY}/relay/client?token=${encodeURIComponent(token)}`)
    await new Promise((resolve, reject) => {
      ws.addEventListener('open', resolve, { once: true })
      ws.addEventListener('error', () => reject(new Error('socket error')), { once: true })
    })
    return new Client(ws)
  }

  send(frame) {
    this.ws.send(JSON.stringify(frame))
  }

  next(timeoutMs = 15000) {
    if (this.inbox.length) return Promise.resolve(this.inbox.shift())
    return new Promise((resolve, reject) => {
      const t = setTimeout(() => reject(new Error('timeout waiting for a frame')), timeoutMs)
      this.waiters.push((f) => {
        clearTimeout(t)
        resolve(f)
      })
    })
  }

  async waitFor(what, pred, timeoutMs = 15000) {
    const deadline = Date.now() + timeoutMs
    while (Date.now() < deadline) {
      const f = await this.next(Math.max(1, deadline - Date.now()))
      if (pred(f)) return f
    }
    throw new Error(`timed out waiting for ${what}`)
  }

  inner(stream, msg) {
    this.send({ t: 'data', stream, payload: Buffer.from(JSON.stringify(msg)).toString('base64') })
  }
}

const decodeInner = (f) => JSON.parse(Buffer.from(f.payload, 'base64').toString())

async function main() {
  // 1. Mint a credential, the way an operator would.
  const enroll = spawn(join('bin', 'tfrelay'), ['enroll', '-devices', devices, '-name', 'acceptance box'])
  let enrollOut = ''
  enroll.stdout.on('data', (d) => (enrollOut += d))
  const enrollCode = await new Promise((r) => enroll.on('exit', r))
  check('enroll exits cleanly', enrollCode === 0, `exit ${enrollCode}`)

  const idMatch = enrollOut.match(/enrolled (\S+)/)
  const secretMatch = enrollOut.match(/-device-secret (\S+)/)
  check('enroll prints a device id', Boolean(idMatch), enrollOut.trim())
  check('enroll prints a secret', Boolean(secretMatch))

  const deviceId = idMatch?.[1]
  const deviceSecret = secretMatch?.[1]
  if (!deviceId || !deviceSecret) throw new Error('enrollment did not produce a usable credential')

  const registry = JSON.parse(readFileSync(devices, 'utf8'))
  check('the registry holds the credential', Boolean(registry.devices?.[deviceId]?.secret))

  // 2. The relay, then a daemon that dials out to it.
  launch(join('bin', 'tfrelay'), [
    '-addr', RELAY,
    '-devices', devices,
    '-client-token', TOKEN,
  ])
  await waitFor(
    async () => {
      const r = await fetch(`http://${RELAY}/relay/machines?token=${TOKEN}`)
      return r.ok
    },
    'relay to answer',
  )

  launch(join('bin', 'tfd'), [
    '-addr', `127.0.0.1:${DAEMON_PORT}`,
    '-relay', `ws://${RELAY}/relay/agent`,
    '-device-id', deviceId,
    '-device-secret', deviceSecret,
    '-machine-name', 'acceptance box',
  ])

  const machines = await waitFor(async () => {
    const r = await fetch(`http://${RELAY}/relay/machines?token=${TOKEN}`)
    const body = await r.json()
    return body.machines?.find((m) => m.online) ? body.machines : null
  }, 'the member to come online')

  const member = machines.find((m) => m.id === deviceId)
  check('the member registered with the relay', Boolean(member), JSON.stringify(machines))
  check('the member reports its name', member?.name === 'acceptance box', member?.name)
  check('the member reports its platform', Boolean(member?.host?.platform), JSON.stringify(member?.host))

  // 3. A client drives a real shell through the relay.
  const c = await Client.dial(TOKEN)
  const list = await c.waitFor('machine list', (f) => f.t === 'machines')
  check('a client receives the machine list', list.machines.some((m) => m.id === deviceId && m.online))

  const STREAM = 1
  c.send({ t: 'open', machine: deviceId, stream: STREAM })
  const acc = await c.waitFor('accept', (f) => f.t === 'accept' && f.stream === STREAM)
  check('open is accepted', acc.machine === deviceId)

  c.inner(STREAM, { t: 'hello', v: 1 })
  const hello = decodeInner(await c.waitFor('inner hello', (f) => f.t === 'data' && decodeInner(f).t === 'hello'))
  check('inner handshake carries the host', Boolean(hello.host?.platform))

  c.inner(STREAM, { t: 'create', sessionId: 's1', cols: 80, rows: 24 })
  const awaited = []
  const collect = async (what, pred) => {
    const f = await c.waitFor(what, (f) => f.t === 'data' && pred(decodeInner(f)))
    awaited.push(decodeInner(f))
    return awaited[awaited.length - 1]
  }
  await collect('created', (m) => m.t === 'created')
  const attached = await collect('attached', (m) => m.t === 'attached')
  check('the session is alive through the relay', attached.session?.alive === true)
  const role = await collect('role', (m) => m.t === 'role')
  check('the remote client controls the session', role.role === 'controller', role.role)

  c.inner(STREAM, { t: 'input', sessionId: 's1', data: Buffer.from(`echo ${MARKER}\n`).toString('base64') })

  let seen = ''
  const deadline = Date.now() + 20000
  while (!seen.includes(MARKER) && Date.now() < deadline) {
    const f = await c.waitFor('output', (f) => f.t === 'data' && decodeInner(f).t === 'output')
    seen += Buffer.from(decodeInner(f).data ?? '', 'base64').toString()
  }
  check('a real shell answers through the relay', seen.includes(MARKER), seen.slice(-200))

  // 4. A second stream must not share state with the first.
  const S2 = 2
  c.send({ t: 'open', machine: deviceId, stream: S2 })
  await c.waitFor('accept on stream 2', (f) => f.t === 'accept' && f.stream === S2)
  c.inner(S2, { t: 'hello', v: 1 })
  await c.waitFor('inner hello on stream 2', (f) => {
    if (f.t !== 'data' || f.stream !== S2) return false
    return decodeInner(f).t === 'hello'
  })
  c.inner(S2, { t: 'create', sessionId: 's2', cols: 100, rows: 30 })
  await c.waitFor('created on stream 2', (f) => f.t === 'data' && f.stream === S2 && decodeInner(f).t === 'created')
  const listFrames = []
  c.inner(S2, { t: 'list', reqId: 'r1' })
  await c.waitFor('session list', (f) => {
    if (f.t !== 'data' || f.stream !== S2) return false
    const m = decodeInner(f)
    if (m.t !== 'sessions') return false
    listFrames.push(m)
    return true
  })
  const sessions = listFrames[0].sessions ?? []
  check('stream 2 sees both sessions', sessions.length === 2, JSON.stringify(sessions.map((s) => s.sessionId)))
  check('stream 2 did not disturb stream 1', sessions.some((s) => s.sessionId === 's1' && s.alive))

  // 5. Talking to a machine that is not there is an answer, not a hang.
  const c2 = await Client.dial(TOKEN)
  c2.send({ t: 'open', machine: 'nope', stream: 1 })
  const off = await c2.waitFor('offline error', (f) => f.t === 'error')
  check('an unknown machine is refused', off.code === 'machine_offline', off.code)

  // 6. The token is actually checked.
  let rejected = false
  try {
    const bad = new WebSocket(`ws://${RELAY}/relay/client?token=wrong`)
    await new Promise((resolve, reject) => {
      bad.addEventListener('open', () => resolve(), { once: true })
      bad.addEventListener('error', () => reject(new Error('rejected')), { once: true })
      setTimeout(() => reject(new Error('no answer')), 5000)
    })
  } catch {
    rejected = true
  }
  check('a wrong client token is rejected', rejected)
}

main()
  .then(() => {
    console.log(`\n${pass} passed, ${fail} failed`)
    cleanup()
    process.exit(fail === 0 ? 0 : 1)
  })
  .catch((err) => {
    console.error(`\nfatal: ${err.message}`)
    cleanup()
    process.exit(1)
  })