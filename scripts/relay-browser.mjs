// Headless acceptance run for the M3 client: a real browser driving a real
// relay, with a real member behind it.
//
//   pnpm run acceptance:relay-browser
//
// Env: TFD, TFRELAY, CHROME, RELAY_PORT, MEMBER_PORT
//
// This is the only check that exercises the code the phone actually runs. The
// scripted relay run (relay-acceptance.mjs) proves the tunnel routes bytes; it
// cannot prove the browser speaks the tunnel. That is what this does, and it
// does it twice: once as a desktop, once emulating a phone.

import { spawn } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const CHROME = process.env.CHROME ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const TFD = process.env.TFD ?? 'bin/tfd'
const TFRELAY = process.env.TFRELAY ?? 'bin/tfrelay'
const RELAY_PORT = Number(process.env.RELAY_PORT ?? 7697)
const MEMBER_PORT = Number(process.env.MEMBER_PORT ?? 7677)
const DEBUG_PORT = 9334
const TOKEN = 'relay-browser-token'
const MARKER = 'M3_CHAIN_OK'
const FROM_PHONE = 'M3_FROM_PHONE'

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? `  — ${detail}` : ''}`)
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

async function waitFor(label, fn, timeout = 20000, step = 120) {
  const deadline = Date.now() + timeout
  let last
  for (;;) {
    try {
      const v = await fn()
      if (v) return v
      last = v
    } catch (err) {
      last = err
    }
    if (Date.now() > deadline) throw new Error(`timeout waiting for ${label}: ${last}`)
    await sleep(step)
  }
}

// ------------------------------------------------------------------ CDP page

class Page {
  constructor(ws) {
    this.ws = ws
    this.nextId = 1
    this.pending = new Map()
    this.logs = []
    ws.addEventListener('message', (ev) => {
      const msg = JSON.parse(ev.data)
      // Browser-side failures are invisible from here otherwise, and the whole
      // point of this run is to catch them.
      if (msg.method === 'Runtime.exceptionThrown') {
        const d = msg.params?.exceptionDetails
        this.logs.push(`exception: ${d?.exception?.description ?? d?.text}`)
      }
      if (msg.method === 'Runtime.consoleAPICalled') {
        const text = (msg.params?.args ?? [])
          .map((a) => a.value ?? a.description ?? a.type)
          .join(' ')
        this.logs.push(`${msg.params?.type}: ${text}`)
      }
      if (msg.id && this.pending.has(msg.id)) {
        const { resolve, reject } = this.pending.get(msg.id)
        this.pending.delete(msg.id)
        if (msg.error) reject(new Error(msg.error.message))
        else resolve(msg.result)
      }
    })
  }

  static async open(url, { mobile = false } = {}) {
    const target = await (
      await fetch(`http://127.0.0.1:${DEBUG_PORT}/json/new?${encodeURIComponent('about:blank')}`, {
        method: 'PUT',
      })
    ).json()
    const ws = new WebSocket(target.webSocketDebuggerUrl)
    await new Promise((res, rej) => {
      ws.addEventListener('open', res, { once: true })
      ws.addEventListener('error', rej, { once: true })
    })
    const page = new Page(ws)
    page.targetId = target.id
    await page.cdp('Runtime.enable')
    await page.cdp('Page.enable')
    // Installed before navigation so the app's own code sees it: taking a shell
    // from a live holder is confirmed, and a modal dialog would otherwise block
    // the page. What it asked is recorded, so a check can assert the question
    // named the holder.
    await page.cdp('Page.addScriptToEvaluateOnNewDocument', {
      source: `
        window.__confirmed = [];
        window.confirm = (message) => { window.__confirmed.push(String(message)); return true; };
      `,
    })
    if (mobile) {
      // Both halves matter: the metrics produce the narrow layout, the media
      // feature produces `(pointer: coarse)`, which is what decides whether the
      // touch key bar and the drawer exist at all.
      await page.cdp('Emulation.setDeviceMetricsOverride', {
        width: 390,
        height: 844,
        deviceScaleFactor: 3,
        mobile: true,
      })
      await page.cdp('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 5 })
      await page.cdp('Emulation.setEmulatedMedia', {
        features: [{ name: 'pointer', value: 'coarse' }],
      })
    }
    await page.cdp('Page.navigate', { url })
    return page
  }

  cdp(method, params = {}) {
    const id = this.nextId++
    this.ws.send(JSON.stringify({ id, method, params }))
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject })
      setTimeout(() => {
        if (this.pending.has(id)) {
          this.pending.delete(id)
          reject(new Error(`CDP timeout: ${method}`))
        }
      }, 20000)
    })
  }

  async evaluate(expression) {
    const r = await this.cdp('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true })
    if (r.exceptionDetails) {
      throw new Error(
        `evaluate failed: ${r.exceptionDetails.exception?.description ?? r.exceptionDetails.text}`,
      )
    }
    return r.result.value
  }

  /** Send a command through the composer: the terminal takes no direct input. */
  async send(text) {
    await this.evaluate(`document.querySelector('.tf-composer-input').focus(); true`)
    await this.evaluate(
      `document.querySelector('.tf-composer-input').value = ${JSON.stringify(text)}; true`,
    )
    for (const type of ['keyDown', 'keyUp']) {
      await this.cdp('Input.dispatchKeyEvent', {
        type,
        key: 'Enter',
        code: 'Enter',
        text: type === 'keyDown' ? '\r' : undefined,
        unmodifiedText: type === 'keyDown' ? '\r' : undefined,
        windowsVirtualKeyCode: 13,
        nativeVirtualKeyCode: 13,
      })
    }
  }

  /** What xterm has actually painted, as one string. */
  rows() {
    return this.evaluate(`[...document.querySelectorAll('.tf-pane.active .xterm-rows')].map((n) => n.textContent).join('\\n')`)
  }

  async close() {
    try {
      if (this.targetId) await fetch(`http://127.0.0.1:${DEBUG_PORT}/json/close/${this.targetId}`)
    } catch {
      /* best effort */
    }
    this.ws.close()
  }
}

// ---------------------------------------------------------------------- main

function start(cmd, args) {
  const p = spawn(cmd, args, { stdio: ['ignore', 'pipe', 'pipe'] })
  p.out = ''
  p.err = ''
  p.stdout.on('data', (b) => (p.out += b.toString()))
  p.stderr.on('data', (b) => (p.err += b.toString()))
  return p
}

const work = mkdtempSync(join(tmpdir(), 'tf-relay-browser-'))
const devices = join(work, 'devices.json')
const profile = join(work, 'chrome-profile')

let chrome
let relay
let member
let desktopPage
let phonePage
let code = 0

try {
  // 1. Enroll a device, so the member has a credential to present.
  const enroll = start(TFRELAY, ['enroll', '-devices', devices, '-name', 'mac-mini'])
  await new Promise((r) => enroll.on('exit', r))
  const deviceId = enroll.out.match(/enrolled (\S+)/)?.[1]
  const deviceSecret = enroll.out.match(/-device-secret (\S+)/)?.[1]
  if (!deviceId || !deviceSecret) throw new Error(`enroll failed: ${enroll.out}${enroll.err}`)
  check('a device credential was enrolled', true, deviceId)

  // 2. Relay up.
  relay = start(TFRELAY, ['-addr', `127.0.0.1:${RELAY_PORT}`, '-devices', devices, '-client-token', TOKEN])
  await waitFor('relay to listen', async () => (await fetch(`http://127.0.0.1:${RELAY_PORT}/healthz`)).ok)

  // 3. The relay serves the client, and says which transport it wants.
  const cfg = await (await fetch(`http://127.0.0.1:${RELAY_PORT}/tf/config`)).json()
  check('the relay reports relay mode', cfg.mode === 'relay', JSON.stringify(cfg))

  // 4. Member dials out — no inbound port, which is the whole point.
  member = start(TFD, [
    '-addr', `127.0.0.1:${MEMBER_PORT}`,
    '-relay', `ws://127.0.0.1:${RELAY_PORT}/relay/agent`,
    '-device-id', deviceId,
    '-device-secret', deviceSecret,
  ])
  await waitFor('the member to appear online', async () => {
    const r = await fetch(`http://127.0.0.1:${RELAY_PORT}/relay/machines`, {
      headers: { Authorization: `Bearer ${TOKEN}` },
    })
    const body = await r.json()
    return body.machines?.some((m) => m.online)
  })

  // 5. Chrome.
  chrome = spawn(
    CHROME,
    [
      '--headless=new',
      `--remote-debugging-port=${DEBUG_PORT}`,
      `--user-data-dir=${profile}`,
      '--no-first-run',
      '--no-default-browser-check',
      '--disable-gpu',
      'about:blank',
    ],
    { stdio: ['ignore', 'ignore', 'ignore'] },
  )
  await waitFor('chrome to be ready', async () => {
    try {
      return (await fetch(`http://127.0.0.1:${DEBUG_PORT}/json/version`)).ok
    } catch {
      return false
    }
  })

  const APP = `http://127.0.0.1:${RELAY_PORT}/`

  // ---------------------------------------------------------------- desktop

  const desktop = (desktopPage = await Page.open(APP))

  // The relay serves a UI that cannot talk to it until it is given a token.
  await waitFor('the connect screen', () =>
    desktop.evaluate(`Boolean(document.querySelector('.tf-connect .tf-input'))`),
  )
  check('the client asks for a token when served by the relay', true)

  await desktop.evaluate(`
    (() => {
      const inputs = document.querySelectorAll('.tf-connect .tf-input')
      inputs[1].value = ${JSON.stringify(TOKEN)}
      document.querySelector('.tf-connect .tf-btn-primary').click()
      return true
    })()
  `)

  await waitFor('the connect screen to go away', () =>
    desktop.evaluate(`!document.querySelector('.tf-connect')`),
  )
  check('the token is accepted and the screen closes', true)

  await waitFor('the machine to be listed', () =>
    desktop.evaluate(`Boolean(document.querySelector('.tf-machine.online, .tf-machine:not(.off)'))`),
  )
  const machineName = await desktop.evaluate(
    `document.querySelector('.tf-machine-name')?.textContent ?? ''`,
  )
  check('the member shows up in the roster', machineName.length > 0, machineName)

  await waitFor('the connection to report online', async () => {
    const chip = await desktop.evaluate(`document.querySelector('.tf-chip')?.textContent ?? ''`)
    return chip.includes('已连接')
  })
  check('the browser reports a live link through the relay', true)

  // Create a terminal and use it. Nothing below this line would work if the
  // browser's tunnel framing were wrong.
  await desktop.evaluate(`document.querySelector('.tf-tab-add').click(); true`)
  await waitFor('a session to be created', () =>
    desktop.evaluate(`Boolean(document.querySelector('.tf-session[data-session-id]'))`),
  )
  const sessionId = await desktop.evaluate(
    `document.querySelector('.tf-session')?.dataset.sessionId ?? ''`,
  )
  check('creating a terminal created a host session', sessionId.length > 0, sessionId)

  await waitFor('the shell prompt', async () => (await desktop.rows()).includes('$'))
  check('the shell prompt renders through the relay', true)

  await desktop.send(`echo ${MARKER}`)
  await waitFor('the marker to come back', async () => (await desktop.rows()).includes(MARKER))
  check('input and output survive the round trip', true)

  const desktopRole = await desktop.evaluate(`document.querySelector('.tf-status')?.textContent ?? ''`)
  check('the first client holds control', desktopRole.includes('控制中'), desktopRole)

  // ------------------------------------------------------------------ phone

  const phone = (phonePage = await Page.open(APP, { mobile: true }))

  // A phone is a different device: it has never been given a token. The two
  // pages here share one Chrome profile, so clear what the desktop stored —
  // otherwise this would silently test the "already trusted device" path.
  await phone.evaluate(`localStorage.clear(); location.reload(); true`)

  await waitFor('the phone connect screen', () =>
    phone.evaluate(`Boolean(document.querySelector('.tf-connect .tf-input'))`),
  )
  check('a phone with no stored token is asked for one', true)
  await phone.evaluate(`
    (() => {
      const inputs = document.querySelectorAll('.tf-connect .tf-input')
      inputs[1].value = ${JSON.stringify(TOKEN)}
      document.querySelector('.tf-connect .tf-btn-primary').click()
      return true
    })()
  `)

  await waitFor('the phone roster', () =>
    phone.evaluate(`Boolean(document.querySelector('.tf-machine'))`),
  )

  // The session the desktop created must be the same one, by id. This is the
  // "one tab, one real session" promise, checked across two devices.
  const phoneSees = await waitFor('the same session id on the phone', async () => {
    const id = await phone.evaluate(
      `document.querySelector('.tf-session')?.dataset.sessionId ?? ''`,
    )
    return id === sessionId ? id : null
  })
  check('the phone lists the same session id as the desktop', phoneSees === sessionId, phoneSees)

  const keysVisible = await phone.evaluate(
    `getComputedStyle(document.querySelector('.tf-keys')).display !== 'none'`,
  )
  check('the touch key bar appears on a phone', keysVisible)

  const drawerHidden = await phone.evaluate(`
    document.querySelector('.tf-side').getBoundingClientRect().right <= 1
  `)
  check('the sidebar starts off-screen on a phone', drawerHidden)

  await phone.evaluate(`document.querySelector('.tf-side-toggle').click(); true`)
  await sleep(400)
  const drawerOpen = await phone.evaluate(`
    document.querySelector('.tf-side').getBoundingClientRect().left >= -1
  `)
  check('the sidebar toggle opens the drawer', drawerOpen)

  // Attach from the phone: it must replay the ring, so the marker the desktop
  // produced is on screen again.
  await phone.evaluate(`document.querySelector('.tf-session').click(); true`)
  await waitFor('the replayed output on the phone', async () => (await phone.rows()).includes(MARKER))
  check('attaching from the phone replays the session output', true)

  // A phone must not be able to type into a terminal the desktop controls.
  const phoneRole = await phone.evaluate(
    `document.querySelector('.tf-banner')?.textContent ?? ''`,
  )
  check('the phone is read-only while the desktop holds control', phoneRole.includes('只读'), phoneRole)

  // A read-only viewer must not be able to type, and the key bar is not a way
  // around that. The shell echoes what it is given, so the pane is the witness.
  const tapTilde = `
    (() => {
      const key = [...document.querySelectorAll('.tf-key')].find((b) => b.textContent === '~')
      key.click()
      return true
    })()
  `
  await phone.evaluate(tapTilde)
  await sleep(900)
  check('a touch key is ignored while the phone is read-only', !(await phone.rows()).includes('~'))

  // Taking over from the phone must work, and must demote the desktop. It asks
  // first — taking a shell from under someone is a thing a person asks for — so
  // the dialog is answered here and what it asked is checked below.
  await phone.evaluate(`
    (() => {
      const b = [...document.querySelectorAll('.tf-banner button')].find((x) => x.textContent === '接管')
      if (b) b.click()
      return Boolean(b)
    })()
  `)
  await waitFor('the steal to be confirmed', async () => (await phone.evaluate(`window.__confirmed ?? []`)).length > 0)
  const asked = await phone.evaluate(`window.__confirmed ?? []`)
  check('接管前会确认，并说明从谁手里接管', asked.length === 1 && /c\d/.test(asked[0]), JSON.stringify(asked))
  await waitFor('the phone to hold control', async () => {
    const s = await phone.evaluate(`document.querySelector('.tf-status')?.textContent ?? ''`)
    return s.includes('控制中')
  })
  check('the phone can take over a session', true)

  // Now that the phone owns the session, the accessory keys are the whole
  // reason a phone is usable: tap `~` and the shell's own echo shows it.
  await phone.evaluate(tapTilde)
  await waitFor('the touch key to reach the shell', async () => (await phone.rows()).includes('~'))
  check('a touch key sends a real keystroke once the phone holds control', true)

  await phone.send(`echo ${FROM_PHONE}`)
  await waitFor('the phone output', async () => (await phone.rows()).includes(FROM_PHONE))
  check('the phone can drive the host session', true)

  const demoted = await waitFor('the desktop to be demoted', async () => {
    const s = await desktop.evaluate(`document.querySelector('.tf-banner')?.textContent ?? ''`)
    return s.includes('只读') ? s : null
  })
  check('the desktop is demoted when the phone takes over', demoted.includes('只读'), demoted)

  // A session that only the relay knows about must survive the desktop going
  // away: the process lives on the member, not in the client.
  await desktop.cdp('Page.navigate', { url: 'about:blank' })
  await sleep(600)
  const survivor = await phone.evaluate(`document.querySelector('.tf-status')?.textContent ?? ''`)
  check('the session outlives the client that opened it', survivor.includes(sessionId), survivor)

  await desktop.close()
  await phone.close()
} catch (err) {
  check('run completed', false, String(err && err.message ? err.message : err))
  code = 1
} finally {
  for (const p of [desktopPage, phonePage]) {
    if (!p) continue
    if (p.logs?.length) console.log(`\n[page logs]\n${p.logs.join('\n')}`)
    try {
      const state = await p.evaluate(`
        JSON.stringify({
          chip: document.querySelector('.tf-chip')?.textContent ?? null,
          status: document.querySelector('.tf-status')?.textContent ?? null,
          side: document.querySelector('.tf-side-list')?.textContent ?? null,
          banner: document.querySelector('.tf-banner')?.textContent ?? null,
        })
      `)
      console.log(`[page state] ${state}`)
    } catch {
      /* the page may be gone */
    }
  }
  for (const p of [chrome, member, relay]) {
    if (p && !p.killed) p.kill('SIGKILL')
  }
  await sleep(200)
  rmSync(work, { recursive: true, force: true })
}

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} checks passed`)
if (failed.length) {
  console.log('\nfailures:')
  for (const f of failed) console.log(`  ${f.name}${f.detail ? ` — ${f.detail}` : ''}`)
  if (member?.err) console.log(`\nmember stderr:\n${member.err}`)
  if (relay?.err) console.log(`\nrelay stderr:\n${relay.err}`)
  process.exit(1)
}
process.exit(code)
