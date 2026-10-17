// Repro for the two reported bugs, driven against a real relay with two real
// browser clients — the same shape as relay-browser.mjs:
//
//   bug 1  ending a session from the web client also deletes its tab
//   bug 2  a client that takes control of a session loses its tab immediately
//
//   node scripts/repro-bugs.mjs
//
// Env: RELAY_PORT, MEMBER_PORT, CHROME, DEBUG_PORT

import { spawn } from 'node:child_process'
import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'path'

const CHROME =
  process.env.CHROME ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const RELAY_PORT = Number(process.env.RELAY_PORT ?? 7755)
const MEMBER_PORT = Number(process.env.MEMBER_PORT ?? 7756)
const DEBUG_PORT = Number(process.env.DEBUG_PORT ?? 9334)
const RELAY = `127.0.0.1:${RELAY_PORT}`
const MEMBER = `127.0.0.1:${MEMBER_PORT}`
const TOKEN = 'repro-token'
const APP = `http://${RELAY}/`

const results = []
const check = (name, ok, detail = '') => {
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

// A process that says why it died, so a harness failure is diagnosable rather
// than a bare "fetch failed".
const watch = (name, p) => {
  let out = ''
  p.stdout?.on('data', (d) => (out += d))
  p.stderr?.on('data', (d) => (out += d))
  p.on('exit', (code) => {
    if (code !== 0 && code !== null) console.error(`# ${name} exited ${code}\n${out}`)
  })
  return () => out
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
      if (msg.method === 'Runtime.exceptionThrown') {
        const d = msg.params?.exceptionDetails
        this.logs.push(`exception: ${d?.exception?.description ?? d?.text}`)
      }
      if (msg.method === 'Runtime.consoleAPICalled') {
        const text = (msg.params?.args ?? []).map((a) => a.value ?? a.description ?? a.type).join(' ')
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

  static async open(url) {
    const target = await (
      await fetch(
        `http://127.0.0.1:${DEBUG_PORT}/json/new?${encodeURIComponent('about:blank')}`,
        { method: 'PUT' },
      )
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
    // Answer every confirm() with true, in this frame and every one after a
    // reload: taking a shell from a live holder is confirmed, and a modal
    // dialog would otherwise block the page — and every later evaluate with it.
    await page.cdp('Page.addScriptToEvaluateOnNewDocument', {
      source: 'window.confirm = () => true',
    })
    await page.navigate(url)
    return page
  }

  async cdp(method, params = {}) {
    const id = this.nextId++
    const p = new Promise((resolve, reject) => this.pending.set(id, { resolve, reject }))
    this.ws.send(JSON.stringify({ id, method, params }))
    return p
  }

  async navigate(url) {
    await this.cdp('Page.navigate', { url })
    await waitFor('the page to load', async () => (await this.evaluate('document.readyState')) === 'complete')
  }

  async evaluate(expr, timeoutMs = 10000) {
    const r = await Promise.race([
      this.cdp('Runtime.evaluate', {
        expression: expr,
        returnByValue: true,
        awaitPromise: true,
      }),
      new Promise((_, rej) =>
        setTimeout(() => rej(new Error(`evaluate timed out: ${expr.slice(0, 80)}`)), timeoutMs),
      ),
    ])
    if (r.exceptionDetails) {
      throw new Error(r.exceptionDetails.exception?.description ?? r.exceptionDetails.text)
    }
    return r.result.value
  }

  async close() {
    try {
      await fetch(`http://127.0.0.1:${DEBUG_PORT}/json/close/${this.targetId}`)
    } catch {
      /* the browser may be gone already */
    }
  }
}

// What a page shows, in one object. Everything the checks need.
const state = (page) =>
  page.evaluate(`(() => {
    const q = (s) => document.querySelector(s)
    const tabs = [...document.querySelectorAll('.tf-tab')].map((t) => ({
      label: t.querySelector('.tf-tab-label')?.textContent ?? '',
      cls: t.className,
    }))
    const side = [...document.querySelectorAll('.tf-session')].map((r) => ({
      label: r.querySelector('.tf-session-name')?.textContent ?? '',
      badge: r.querySelector('.tf-session-badge')?.textContent ?? '',
      sid: r.dataset.sessionId ?? '',
    }))
    const banner = q('.tf-banner.on')?.innerText.replace(/\\n/g, ' ') ?? ''
    return {
      status: q('.tf-status')?.innerText.replace(/\\n/g, ' | ') ?? '',
      tabs,
      side,
      banner,
      hero: !!q('.tf-hero') && !q('.tf-hero').hidden,
      composerDisabled: q('.tf-composer-input')?.disabled,
      composerPlaceholder: q('.tf-composer-input')?.placeholder ?? '',
    }
  })()`)

const seedToken = (page) =>
  page.evaluate(`(() => {
    localStorage.setItem('termfleet.relay.token', ${JSON.stringify(TOKEN)})
    localStorage.setItem('termfleet.relay.url', 'ws://${RELAY}/relay/client')
    return true
  })()`)

// ------------------------------------------------------------------ harness

const work = mkdtempSync(join(tmpdir(), 'tf-repro-'))
const procs = []
const launch = (bin, args) => {
  const p = spawn(bin, args, { stdio: ['ignore', 'pipe', 'pipe'] })
  p.stdout.on('data', () => {})
  p.stderr.on('data', () => {})
  procs.push(p)
  return p
}
async function main() {
  console.log(`# relay ${RELAY}  member ${MEMBER}`)
  // Enroll first, then start the relay: the CLI writes the registry file and
  // the relay reads it at startup. (The admin page's live enroll is the path
  // that works while the relay runs; the CLI cannot reach into a running one.)
  const enroll = spawn('./bin/tfrelay', [
    'enroll', '-name', 'repro-member', '-devices', join(work, 'devices.json'),
  ], { stdio: ['ignore', 'pipe', 'pipe'] })
  let enrollOut = ''
  enroll.stdout.on('data', (d) => (enrollOut += d))
  const enrollCode = await new Promise((r) => enroll.on('exit', r))
  const id = enrollOut.match(/enrolled (\S+)/)?.[1]
  const secret = enrollOut.match(/-device-secret (\S+)/)?.[1]
  if (enrollCode !== 0 || !id || !secret) {
    throw new Error(`enrollment failed (exit ${enrollCode}):\n${enrollOut}`)
  }

  watch('relay', launch('./bin/tfrelay', [
    '-addr', RELAY,
    '-devices', join(work, 'devices.json'),
    '-client-token', TOKEN,
  ]))
  await waitFor('the relay', () => fetch(`http://${RELAY}/healthz`).then((r) => r.ok).catch(() => false))

  watch('member', launch('./bin/tfd', [
    '-addr', MEMBER,
    '-relay', `ws://${RELAY}/relay/agent`,
    '-device-id', id,
    '-device-secret', secret,
    '-web-dir', 'internal/web/dist',
  ]))
  await waitFor('the member', () =>
    fetch(`http://${MEMBER}/healthz`).then((r) => r.ok).catch(() => false),
  )
  await waitFor('the member to be online at the relay', async () => {
    const j = await fetch(`http://${RELAY}/healthz`).then((r) => r.json()).catch(() => null)
    return j?.online >= 1
  })

  const chrome = spawn(CHROME, [
    `--remote-debugging-port=${DEBUG_PORT}`,
    '--user-data-dir=' + join(work, 'chrome'),
    '--no-first-run',
    '--no-default-browser-check',
    'about:blank',
  ], { stdio: 'ignore' })
  procs.push(chrome)
  // Chrome takes a moment to open its debugging socket; opening a page before
  // that is a connection refusal that reads like the app itself is broken.
  await waitFor('the browser to answer on its debug port', () =>
    fetch(`http://127.0.0.1:${DEBUG_PORT}/json/version`).then((r) => r.ok).catch(() => false),
  )

  const A = await Page.open(APP)
  const B = await Page.open(APP)
  await seedToken(A)
  await seedToken(B)
  await A.navigate(APP)
  await B.navigate(APP)

  await waitFor('A to be connected', async () => (await state(A)).status.includes('已连接'))
  await waitFor('B to be connected', async () => (await state(B)).status.includes('已连接'))

  // A opens a shell and holds it.
  await A.evaluate(`document.querySelector('.tf-btn-primary').click(); true`)
  await waitFor('a session to exist', () =>
    A.evaluate(`Boolean(document.querySelector('.tf-session[data-session-id]'))`),
  )
  const sid = await A.evaluate(
    `document.querySelector('.tf-session')?.dataset.sessionId ?? ''`,
  )
  check('A created a session', sid.length > 0, sid)
  let sa = await state(A)
  check('A holds control', sa.tabs.length === 1 && sa.status.includes('控制中'), sa.status)

  // B attaches to that same session: it must arrive as a viewer, tab intact.
  await B.evaluate(`document.querySelector('.tf-session')?.click(); true`)
  await waitFor('B to show the session', async () => (await state(B)).tabs.length === 1)
  await sleep(500)
  let sb = await state(B)
  check('B attaches as a viewer', sb.status.includes('只读'), sb.status)
  check('B keeps its tab while watching', sb.tabs.length === 1, JSON.stringify(sb.tabs))

  // ---------------------------------------------------------------- bug 2
  // B takes control. The tab must survive, and B must end up controlling.
  // The offer lives in the header banner. Say what is actually there when it
  // is not found, so a failure here is a diagnosable state and not a timeout.
  const bannerBefore = await B.evaluate(
    `document.querySelector('.tf-banner')?.innerText.replace(/\\n/g,' ') ?? '(no banner)'`,
  )
  const clicked = await B.evaluate(`(() => {
    const btn = [...document.querySelectorAll('.tf-banner button')]
      .find((b) => b.textContent.includes('接管'))
    if (!btn) return false
    btn.click()
    return true
  })()`)
  if (!clicked) throw new Error(`no 接管 button in B's banner: ${bannerBefore}`)
  await sleep(1500)
  sb = await state(B)
  check('B keeps its tab after taking control', sb.tabs.length === 1, JSON.stringify(sb.tabs))
  check('B ends up controlling', sb.status.includes('控制中'), sb.status)
  sa = await state(A)
  check('A keeps its tab after losing control', sa.tabs.length === 1, JSON.stringify(sa.tabs))
  check('A is told it is read-only', sa.status.includes('只读'), sa.status)

  // ------------------------------------------------------- 释放 is its own act
  // B holds the lease now. 释放 gives it up without ending the shell and without
  // closing the tab: the process keeps running, B keeps watching, and the lease
  // goes back to the machine the shell runs on. It used to not exist at all —
  // the frame was in the protocol and no client ever sent it.
  const released = await B.evaluate(`(() => {
    const btn = [...document.querySelectorAll('.tf-banner button')]
      .find((b) => b.textContent.includes('释放'))
    if (!btn) return 'no 释放 button: ' + (document.querySelector('.tf-banner') || {}).innerText
    btn.click()
    return 'clicked'
  })()`)
  check('a client holding the lease is offered 释放', released === 'clicked', released)
  await sleep(1500)
  sb = await state(B)
  check('释放 keeps the tab', sb.tabs.length === 1, JSON.stringify(sb.tabs))
  check('释放 leaves this client read-only', sb.status.includes('只读'), sb.status)
  // And the shell is still alive, so the session is still listed.
  const alive = await B.evaluate(`(() => {
    const rows = [...document.querySelectorAll('.tf-session')]
    return rows.length
  })()`)
  check('释放 did not end the shell', alive === 1, `${alive} sessions listed`)
  // The takeover is offered again, because the lease is free again.
  const canTake = await B.evaluate(
    `[...document.querySelectorAll('.tf-banner button')].some((b) => b.textContent.includes('接管'))`,
  )
  check('接管 is offered again after 释放', canTake === true, String(canTake))

  // Take it back, so the 结束 phase below starts from a client that holds the
  // lease: releasing it was the point of the checks above, and leaving it
  // released would make the next phase about a viewer rather than a controller.
  await B.evaluate(`(() => {
    const btn = [...document.querySelectorAll('.tf-banner button')]
      .find((b) => b.textContent.includes('接管'))
    if (btn) btn.click()
    return true
  })()`)
  await sleep(1500)
  sb = await state(B)
  check('B holds the lease again', sb.status.includes('控制中'), sb.status)

  // ---------------------------------------------------------------- bug 1
  // Ending the session kills the process for everyone, but the tab that was
  // watching it stays so its final output can be read — closing the tab is a
  // separate, explicit action.
  // The killing client keeps its tab and is told the shell exited: the
  // difference between "the shell exited" and "your tab went away" is exactly
  // what the two checks below assert.

  // The row's 结束, which is a different button from the tab's × and from the
  // banner's 关闭标签.
  await A.evaluate(`document.querySelector('.tf-session .tf-kill')?.click(); true`)

  // The exit frame arrives asynchronously, so this waits for the banner to say
  // so rather than sampling the DOM at a fixed moment — the version that slept
  // for a constant passed on a fast machine and failed on a slow one.
  await waitFor('the banner to say the shell exited', async () => {
    const s = await state(A)
    return s.status.includes('已退出') || s.banner.includes('已退出')
  })

  sa = await state(A)
  check('ending a session keeps the tab', sa.tabs.length === 1, JSON.stringify(sa.tabs))
  check('the tab is marked exited', sa.status.includes('已退出') || sa.banner.includes('已退出'), sa.status + ' / ' + sa.banner)
  sb = await state(B)
  check('the other client keeps its tab too', sb.tabs.length === 1, JSON.stringify(sb.tabs))

  const errors = [...A.logs, ...B.logs].filter((l) => l.startsWith('exception'))
  check('no page-level exceptions', errors.length === 0, errors.slice(0, 2).join(' ; '))

  await A.close()
  await B.close()

  console.log(`\n${results.filter((r) => r.ok).length}/${results.length} passed`)
  if (results.some((r) => !r.ok)) process.exitCode = 1
}

main()
  .catch((err) => {
    console.error('repro failed:', err && err.stack ? err.stack : err)
    console.error('cause:', err?.cause?.code, err?.cause?.message, err?.cause?.syscall, err?.cause?.address, err?.cause?.port)
    process.exitCode = 1
  })
  .finally(() => {
    for (const p of procs) p.kill('SIGTERM')
  })
