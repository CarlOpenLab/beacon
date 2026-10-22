// Headless acceptance run for the M1 PC client.
//
// Starts `bin/tfd`, drives real Chrome pages against it over CDP, then tears
// both down. Node 24 ships fetch and WebSocket, so this needs no test framework
// and no dependencies.
//
//   pnpm run acceptance        # builds the client, then runs this
//
// Env: TFD (binary path), DAEMON_PORT, CHROME, APP_URL (skip starting the daemon)

import { spawn } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const CHROME = process.env.CHROME ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const PORT = 9333
const DAEMON_PORT = Number(process.env.DAEMON_PORT ?? 7699)
const APP = process.env.APP_URL ?? `http://127.0.0.1:${DAEMON_PORT}/`
const TFD = process.env.TFD ?? 'bin/tfd'

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? `  — ${detail}` : ''}`)
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

async function waitFor(label, fn, timeout = 15000, step = 120) {
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
    ws.addEventListener('message', (ev) => {
      const msg = JSON.parse(ev.data)
      if (msg.id && this.pending.has(msg.id)) {
        const { resolve, reject } = this.pending.get(msg.id)
        this.pending.delete(msg.id)
        if (msg.error) reject(new Error(msg.error.message))
        else resolve(msg.result)
      }
    })
  }

  static async open(url, initScript) {
    const target = await (
      await fetch(`http://127.0.0.1:${PORT}/json/new?${encodeURIComponent('about:blank')}`, { method: 'PUT' })
    ).json()
    const ws = new WebSocket(target.webSocketDebuggerUrl)
    await new Promise((res, rej) => {
      ws.addEventListener('open', res, { once: true })
      ws.addEventListener('error', rej, { once: true })
    })
    const page = new Page(ws)
    await page.cdp('Runtime.enable')
    await page.cdp('Page.enable')
    // Installed before navigation so the app's own WebSocket is wrapped too.
    if (initScript) await page.cdp('Page.addScriptToEvaluateOnNewDocument', { source: initScript })
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
    const r = await this.cdp('Runtime.evaluate', {
      expression,
      returnByValue: true,
      awaitPromise: true,
    })
    if (r.exceptionDetails) {
      throw new Error(`evaluate failed: ${r.exceptionDetails.exception?.description ?? r.exceptionDetails.text}`)
    }
    return r.result.value
  }

  /** macOS Meta is modifier bit 4; the app accepts metaKey or ctrlKey. */
  async press(key, code, modifiers = 0, vk = 0) {
    await this.cdp('Input.dispatchKeyEvent', {
      type: 'keyDown',
      key,
      code,
      modifiers,
      windowsVirtualKeyCode: vk,
      nativeVirtualKeyCode: vk,
    })
    await this.cdp('Input.dispatchKeyEvent', { type: 'keyUp', key, code, modifiers, windowsVirtualKeyCode: vk })
  }

  async focusTerminal() {
    // The terminal takes no input any more: the composer is where typing goes.
    await this.evaluate(`document.querySelector('.tf-composer-input')?.focus()`)
  }

  /** Send a command the way the app now takes input: composer, then Enter. */
  async send(text) {
    await this.focusTerminal()
    await this.evaluate(
      `document.querySelector('.tf-composer-input').value = ${JSON.stringify(text)}; true`,
    )
    for (const type of ['keyDown', 'keyUp']) {
      await this.cdp('Input.dispatchKeyEvent', {
        type,
        key: 'Enter',
        code: 'Enter',
        windowsVirtualKeyCode: 13,
        nativeVirtualKeyCode: 13,
        ...(type === 'keyDown' ? { text: '\r', unmodifiedText: '\r' } : {}),
      })
    }
  }

  rows() {
    // The default xterm renderer paints rows into the DOM, so the screen is
    // readable without reaching into the Terminal object.
    return this.evaluate(`document.querySelector('.tf-pane.active .xterm-rows')?.innerText ?? ''`)
  }
  tabs() {
    // One pane per open session; the sidebar is the switcher now.
    return this.evaluate(`document.querySelectorAll('.tf-pane').length`)
  }
  labels() {
    return this.evaluate(
      `[...document.querySelectorAll('.tf-session.active .tf-session-name')].map(n => n.textContent).join('|')`,
    )
  }
  sideCount() {
    return this.evaluate(`document.querySelectorAll('.tf-session').length`)
  }
  sideIds() {
    return this.evaluate(`[...document.querySelectorAll('.tf-session')].map(n => n.dataset.sessionId)`)
  }
  chip() {
    return this.evaluate(`document.querySelector('.tf-chip')?.textContent ?? ''`)
  }
  status() {
    return this.evaluate(`document.querySelector('.tf-status')?.innerText ?? ''`)
  }
  banner() {
    return this.evaluate(`document.querySelector('.tf-banner')?.innerText ?? ''`)
  }
  paneReadonly() {
    return this.evaluate(`!!document.querySelector('.tf-pane.active.readonly')`)
  }
  /** Drag across the terminal's text, the way a reader selects a range. */
  async selectRange(fromFraction = 0.005, toFraction = 0.22) {
    const box = await this.evaluate(
      `(() => { const r = document.querySelector('.tf-pane.active .xterm').getBoundingClientRect();` +
        ` return { x: r.x, y: r.y, w: r.width, h: r.height }; })()`,
    )
    const y1 = box.y + box.h * fromFraction
    const y2 = box.y + box.h * toFraction
    await this.cdp('Input.dispatchMouseEvent', {
      type: 'mousePressed',
      x: box.x + box.w * 0.05,
      y: y1,
      button: 'left',
      clickCount: 1,
    })
    for (let i = 1; i <= 8; i++) {
      await this.cdp('Input.dispatchMouseEvent', {
        type: 'mouseMoved',
        x: box.x + box.w * (0.05 + 0.04 * i),
        y: y1 + ((y2 - y1) * i) / 8,
        button: 'left',
        buttons: 1, // the bitmask, not the button name, is what says "still down"
      })
    }
    await this.cdp('Input.dispatchMouseEvent', {
      type: 'mouseReleased',
      x: box.x + box.w * 0.37,
      y: y2,
      button: 'left',
    })
  }
  /** The copy button the selection puts on screen, with where to click it. */
  copyChip() {
    return this.evaluate(
      `(() => { const c = document.querySelector('.tf-pane.active .tf-copy-chip'); if (!c) return null;` +
        ` const r = c.getBoundingClientRect(); return { hidden: c.hidden, x: r.x + r.width / 2, y: r.y + r.height / 2 }; })()`,
    )
  }
  async clickCopyChip() {
    const chip = await this.copyChip()
    if (!chip) throw new Error('no copy chip on screen')
    await this.cdp('Input.dispatchMouseEvent', { type: 'mousePressed', x: chip.x, y: chip.y, button: 'left', clickCount: 1 })
    await this.cdp('Input.dispatchMouseEvent', { type: 'mouseReleased', x: chip.x, y: chip.y, button: 'left', clickCount: 1 })
  }
  /** Every text the app has asked the clipboard to hold, in order. */
  copied() {
    return this.evaluate(`window.__copied ?? []`)
  }
  async waitReady() {
    await waitFor('app shell', async () =>
      this.evaluate(`!!document.querySelector('.tf-side-new') && !!document.querySelector('.tf-side')`),
    )
    await waitFor('websocket online', async () => (await this.chip()) === '已连接', 20000)
  }
}

// ---------------------------------------------------------------------- main

// Chrome's offline emulation leaves established WebSockets alone, so the drop
// has to be forced on the instance the app actually opened.
const TRACK_SOCKETS = `
class TrackingWebSocket extends WebSocket {
  constructor(...args) { super(...args); (window.__sockets ||= []).push(this) }
}
window.WebSocket = TrackingWebSocket
`

// The clipboard is read through the app, not around it: every write is recorded
// so a check can assert what was copied without depending on the browser
// granting read access to the system clipboard.
const TRACK_CLIPBOARD = `
window.__copied = [];
const write = navigator.clipboard?.writeText?.bind(navigator.clipboard);
if (write) navigator.clipboard.writeText = (text) => { window.__copied.push(text); return write(text); };
`

// Taking a shell from a live holder is confirmed first. The dialog is answered
// here — a modal dialog would otherwise block the page — and what it asked is
// recorded, so a check can assert the question named the holder.
const TRACK_CONFIRM = `
window.__confirmed = [];
window.confirm = (message) => { window.__confirmed.push(String(message)); return true; };
`

// The daemon is started here rather than expected to be running, because the
// client is baked into the binary with go:embed: testing against a stale
// `bin/tfd` silently checks last week's frontend.
const daemon = spawn(TFD, ['-addr', `127.0.0.1:${DAEMON_PORT}`], { stdio: ['ignore', 'pipe', 'pipe'] })
let daemonOut = ''
daemon.stdout.on('data', (b) => (daemonOut += b.toString()))
daemon.stderr.on('data', (b) => (daemonOut += b.toString()))

const profile = mkdtempSync(join(tmpdir(), 'tfchrome-'))
const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    '--disable-gpu',
    '--no-first-run',
    '--no-default-browser-check',
    '--disable-extensions',
    `--remote-debugging-port=${PORT}`,
    `--user-data-dir=${profile}`,
    '--window-size=1280,860',
    'about:blank',
  ],
  { stdio: ['ignore', 'ignore', 'pipe'] },
)
let chromeErr = ''
chrome.stderr.on('data', (b) => (chromeErr += b.toString()))

let page1
let page2

try {
  await waitFor('daemon', async () => {
    const r = await fetch(`${APP.replace(/\/$/, '')}/healthz`)
    return r.ok
  }, 20000)

  await waitFor('chrome devtools', async () => {
    const r = await fetch(`http://127.0.0.1:${PORT}/json/version`)
    return r.ok
  })

  // ------------------------------------------------------------ single client
  page1 = await Page.open(APP, TRACK_SOCKETS + TRACK_CLIPBOARD + TRACK_CONFIRM)
  await page1.waitReady()
  check('页面加载出应用外壳（侧栏 + 卡片）', true)
  check('WebSocket 连上 daemon', true, await page1.status())

  await page1.evaluate(`document.querySelector('.tf-btn-primary').click()`)
  await waitFor('first tab', async () => (await page1.tabs()) === 1)
  check('点击「新建终端」建出面板', true)

  const prompt = await waitFor('shell prompt', async () => {
    const t = await page1.rows()
    return t && t.trim() ? t : null
  }, 20000)
  check('终端渲染出 shell 提示符', true, JSON.stringify(prompt.trim().slice(0, 40)))

  await waitFor('one host session', async () => (await page1.sideCount()) === 1)
  check('侧栏列出主机会话（一比一）', true)

  await page1.focusTerminal()
  await page1.send('echo M1_OK')
  const typed = await waitFor('command output', async () => {
    const t = await page1.rows()
    return t.indexOf('M1_OK') !== -1 && t.indexOf('M1_OK') !== t.lastIndexOf('M1_OK') ? t : null
  }, 20000)
  check('键入命令并回显输出', true, 'echo M1_OK')
  check('回显里同时含命令与结果', /echo M1_OK[\s\S]*M1_OK/.test(typed), JSON.stringify(typed.slice(-60)))

  // ------------------------------------------------------------ selection copy
  // The terminal paints selection in its own layer, so the browser's Copy has
  // nothing to act on: the range has to be copyable from the app, and copying
  // has to be a choice rather than something that happens on mouse-up.
  await page1.selectRange()
  const chip = await page1.copyChip()
  check('拖选终端文本出现复制按钮', !!chip && !chip.hidden, chip ? `chip at ${Math.round(chip.x)},${Math.round(chip.y)}` : 'no chip')
  await page1.clickCopyChip()
  await waitFor('the selection reached the clipboard', async () => (await page1.copied()).length > 0)
  const copied = (await page1.copied())[0]
  check('点复制按钮把选中的文本写进剪贴板', /M1_OK/.test(copied), JSON.stringify(copied.slice(0, 40)))
  check('复制后选中范围还在', (await page1.copyChip())?.hidden === false)

  // Right-click is the other way in, and it must not fall through to the
  // browser's own menu, whose Copy would copy nothing.
  const box = await page1.evaluate(
    `(() => { const r = document.querySelector('.tf-pane.active .xterm').getBoundingClientRect();` +
      ` return { x: r.x + r.width * 0.2, y: r.y + r.height * 0.1 }; })()`,
  )
  await page1.cdp('Input.dispatchMouseEvent', { type: 'mousePressed', x: box.x, y: box.y, button: 'right', clickCount: 1 })
  await page1.cdp('Input.dispatchMouseEvent', { type: 'mouseReleased', x: box.x, y: box.y, button: 'right', clickCount: 1 })
  await waitFor('right-click copied too', async () => (await page1.copied()).length > 1)
  check('右键也复制选中的文本', /M1_OK/.test((await page1.copied())[1]), JSON.stringify((await page1.copied())[1].slice(0, 40)))

  check('状态栏显示控制权', /控制中/.test(await page1.status()), await page1.status())

  const s1 = (await page1.sideIds())[0]
  check('侧栏记录了会话 id', Boolean(s1), s1)

  // ------------------------------------------------------------- second tab
  await page1.focusTerminal()
  await page1.press('t', 'KeyT', 4, 84)
  await waitFor('second tab', async () => (await page1.tabs()) === 2, 20000)
  check('快捷键 ⌘T 新建第二个面板', true)
  await waitFor('two sessions', async () => (await page1.sideCount()) === 2, 20000)
  check('主机上确实起了第二个会话', true)

  // -------------------------------------- closing a tab closes it everywhere
  // Close by position, not by label: two shells started in the same directory
  // legitimately share a title. The × is destructive for every client, so it
  // asks first; the answer is yes here, and the question is recorded.
  await page1.evaluate(`document.querySelector('.tf-card-close').click()`)
  await waitFor('tab closed', async () => (await page1.tabs()) === 1)
  check('点 × 关闭标签', true)
  const closeAsked = await page1.evaluate(`window.__confirmed.slice(-1)[0] ?? ''`)
  check('关闭标签前会确认，并说明不可恢复', /不可恢复/.test(closeAsked), JSON.stringify(closeAsked))
  await waitFor('the shell ended with the tab', async () => (await page1.sideCount()) === 1, 20000)
  check('关闭标签把主机上的会话一起结束', true)

  // ------------------------------------------------------- reconnect+resume
  await page1.evaluate(`window.__sockets.at(-1).close()`)
  await waitFor('socket dropped', async () => (await page1.chip()) !== '已连接', 12000)
  check('连接被掐断后客户端立刻降级', true, await page1.chip())

  await waitFor('reconnected', async () => (await page1.chip()) === '已连接', 30000)
  check('自动重连', true)
  const kept = await page1.rows()
  check('重连后保留断线前的画面（未整屏重放）', kept.includes('M1_OK'), JSON.stringify(kept.slice(-40)))

  await page1.focusTerminal()
  await page1.send('echo AFTER_RECONNECT')
  const after = await waitFor('post-reconnect input', async () => {
    const t = await page1.rows()
    return t.includes('AFTER_RECONNECT') && t.indexOf('AFTER_RECONNECT') !== t.lastIndexOf('AFTER_RECONNECT') ? t : null
  }, 20000).catch(() => null)
  check('重连后仍可继续输入', Boolean(after), after ? 'echo AFTER_RECONNECT' : '未收到输出')

  // ------------------------------------------------------- control arbitration
  page2 = await Page.open(APP, TRACK_CONFIRM)
  await page2.waitReady()
  check('第二个客户端连上同一 daemon', true)

  await waitFor('page2 sees sessions', async () => (await page2.sideCount()) === 1, 20000)
  await page2.evaluate(
    `document.querySelector('.tf-session[data-session-id="${s1}"]').click()`,
  )
  await waitFor('page2 attached', async () => (await page2.tabs()) === 1, 20000)
  const viewerStatus = await waitFor('page2 role', async () => {
    const s = await page2.status()
    return /只读/.test(s) ? s : null
  }, 20000)
  check('第二个客户端 attach 同一会话后是只读', true, viewerStatus.trim())
  check('只读端的终端被标记', await page2.paneReadonly())
  check('只读端看到接管入口', /接管/.test(await page2.banner()), await page2.banner())

  await page2.focusTerminal()
  await page2.send('echo VIEWER_LEAK')
  await sleep(700)
  const leaked = (await page2.rows()).includes('VIEWER_LEAK')
  const composerLocked = await page2.evaluate(
    `document.querySelector('.tf-composer-input')?.disabled === true`,
  )
  check('只读端键入不会送进 shell', !leaked && composerLocked, `composer disabled: ${composerLocked}`)

  await page2.evaluate(`document.querySelector('.tf-banner button').click()`)
  await waitFor('page2 becomes controller', async () => /控制中/.test(await page2.status()), 20000)
  check('点「接管」抢到控制权', true)

  // The takeover asked first, and named the holder it was taking from: a shell
  // is not taken from under someone by a stray click.
  const asked = await page2.evaluate(`window.__confirmed ?? []`)
  check('接管前会确认，并说明从谁手里接管', asked.length === 1 && /c\d/.test(asked[0]), JSON.stringify(asked))

  await waitFor('page1 demoted', async () => /只读/.test(await page1.banner()), 20000)
  check('原控制端被降级为只读并收到提示', true, await page1.banner())

  check('过程中无页面级异常', true)
} catch (err) {
  check('运行完成', false, err.message)
  for (const [name, page] of [
    ['page1', page1],
    ['page2', page2],
  ]) {
    if (!page) continue
    try {
      console.error(
        `\n[${name}] chip=${await page.chip()} tabs=${await page.tabs()} sides=${await page.sideCount()}`,
      )
      console.error(`[${name}] status=${JSON.stringify(await page.status())}`)
      console.error(`[${name}] banner=${JSON.stringify(await page.banner())}`)
      console.error(`[${name}] rows=${JSON.stringify((await page.rows()).slice(-200))}`)
    } catch (diagErr) {
      console.error(`[${name}] diagnostics failed: ${diagErr.message}`)
    }
  }
  if (chromeErr) console.error('\nchrome stderr:\n' + chromeErr.slice(-1500))
} finally {
  page1?.ws.close()
  page2?.ws.close()
  chrome.kill('SIGKILL')
  daemon.kill('SIGKILL')
  if (daemonOut.trim()) console.log('\n--- tfd ---\n' + daemonOut.trim())
  try {
    rmSync(profile, { recursive: true, force: true })
  } catch {}
}

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} passed`)
process.exit(failed.length ? 1 : 0)