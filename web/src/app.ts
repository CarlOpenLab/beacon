// The whole PC client. Deliberately framework-free: this bundle is also what the
// phone loads over the LAN in M2, so every kilobyte is paid twice.

import {
  Fleet,
  type ConnState,
  type FleetHandlers,
} from './fleet'
import { ERR, ROLE, type Host, type Role, type Summary } from './protocol'
import { createTerminal, type TerminalBundle } from './term'

/** A tab may exist before the server has told us its session id. */
interface Tab {
  key: number
  sessionId: string
  title: string
  bundle: TerminalBundle
  pane: HTMLElement
  role: Role | null
  alive: boolean
}

const MOD_LABEL = navigator.platform.toLowerCase().includes('mac') ? '⌘' : 'Ctrl'

function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  className?: string,
  text?: string,
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag)
  if (className) node.className = className
  if (text !== undefined) node.textContent = text
  return node
}

function basename(p: string): string {
  if (!p) return ''
  const parts = p.replace(/[/\\]+$/, '').split(/[/\\]/)
  return parts[parts.length - 1] || p
}

/** Clipboard write that still works on plain http, where the async API is absent. */
async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    /* fall through to the legacy path */
  }
  const scratch = el('textarea')
  scratch.value = text
  scratch.setAttribute('readonly', '')
  scratch.style.position = 'fixed'
  scratch.style.top = '-1000px'
  scratch.style.opacity = '0'
  document.body.appendChild(scratch)
  scratch.select()
  let ok = false
  try {
    ok = document.execCommand('copy')
  } catch {
    ok = false
  }
  scratch.remove()
  return ok
}

export class App {
  private fleet: Fleet
  private tabs: Tab[] = []
  private activeKey = -1
  private nextKey = 1

  private root: HTMLElement
  private fleetList!: HTMLElement
  private fleetCount!: HTMLElement
  private tabStrip!: HTMLElement
  private panes!: HTMLElement
  private statusConn!: HTMLElement
  private statusHost!: HTMLElement
  private statusSession!: HTMLElement
  private toastBox!: HTMLElement
  private banner!: HTMLElement

  private sessions: Summary[] = []
  private host?: Host
  private connState: ConnState = 'offline'
  private observer?: ResizeObserver
  private fitTimer?: number

  constructor(container: HTMLElement, endpoint?: string) {
    this.root = container
    this.fleet = new Fleet(this.handlers(), endpoint)
    this.build()
    this.bindKeys()
  }

  start(): void {
    this.fleet.connect()
  }

  // ---------------------------------------------------------------- handlers

  private handlers(): FleetHandlers {
    return {
      state: (s, detail) => {
        this.connState = s
        this.renderStatus()
        if (s === 'offline' && detail) this.toast(detail, 'warn')
      },
      hello: (host, version) => {
        this.host = host
        if (version && version !== 1) {
          this.toast(`服务端协议版本 ${version}，客户端期望 1`, 'warn')
        }
        this.renderStatus()
      },
      sessions: (list) => {
        this.sessions = list
        this.renderFleet()
        // Reflect host-side truth: an exit we missed, or someone else's control.
        for (const tab of this.tabs) {
          if (!tab.sessionId) continue
          const summary = list.find((s) => s.sessionId === tab.sessionId)
          if (!summary) continue
          tab.alive = summary.alive
          if (summary.title) tab.title = summary.title
        }
        this.renderTabs()
      },
      created: (sessionId) => {
        const tab = this.pendingTab()
        if (tab) this.bind(tab, sessionId)
      },
      attached: (sessionId, session, _cursor, truncated) => {
        let tab = this.tabs.find((t) => t.sessionId === sessionId)
        if (!tab) {
          // Attached without a local create: someone asked for an existing session.
          tab = this.openTab(sessionId)
        }
        this.bind(tab, sessionId)
        if (!session) return
        tab.alive = session.alive
        tab.title = session.title || basename(session.cwd) || sessionId
        this.renderTabs()
        if (truncated) {
          // The ring buffer no longer holds everything we missed; the screen we
          // would repaint from is a guess, so start from a clean slate instead.
          tab.bundle.term.reset()
          this.toast(`${tab.title}：断线期间的输出已超出缓冲，已重置画面`, 'warn')
        }
      },
      output: (sessionId, _cursor, data, resync) => {
        const tab = this.tabs.find((t) => t.sessionId === sessionId)
        if (!tab) return
        if (resync) tab.bundle.term.reset()
        if (data.length) tab.bundle.term.write(data)
      },
      role: (sessionId, role, controller) => {
        const tab = this.tabs.find((t) => t.sessionId === sessionId)
        if (!tab) return
        tab.role = role
        tab.bundle.term.options.disableStdin = role !== ROLE.controller
        tab.pane.classList.toggle('readonly', role !== ROLE.controller)
        this.renderTabs()
        this.renderBanner()
        this.renderStatus()
        if (role === ROLE.controller) {
          tab.bundle.term.focus()
        } else if (controller) {
          this.toast(`已被接管（${controller}），你现在是只读`, 'info')
        }
      },
      exited: (sessionId, code) => {
        const tab = this.tabs.find((t) => t.sessionId === sessionId)
        if (!tab) return
        tab.alive = false
        tab.title = `${tab.title} — 已退出`
        tab.bundle.term.write(`\r\n\x1b[2m[进程已退出，退出码 ${code}]\x1b[0m\r\n`)
        tab.bundle.term.options.disableStdin = true
        this.renderTabs()
        this.renderBanner()
      },
      closed: (sessionId) => {
        const tab = this.tabs.find((t) => t.sessionId === sessionId)
        if (tab) this.removeTab(tab)
      },
      errored: (code, message, sessionId) => {
        if (code === ERR.notController) {
          this.toast('该会话由其他端控制，输入已被拒绝', 'warn')
          const tab = this.tabs.find((t) => t.sessionId === sessionId)
          if (tab) {
            tab.role = ROLE.viewer
            tab.bundle.term.options.disableStdin = true
            this.renderBanner()
            this.renderTabs()
          }
          return
        }
        this.toast(`${code}: ${message}`, 'error')
      },
    }
  }

  /** A tab waiting for its `created` frame. */
  private pendingTab(): Tab | undefined {
    return this.tabs.find((t) => t.sessionId === '')
  }

  private bind(tab: Tab, sessionId: string): void {
    if (tab.sessionId === sessionId) return
    tab.sessionId = sessionId
    tab.title = tab.title || sessionId
    this.renderTabs()
  }

  // -------------------------------------------------------------------- DOM

  private build(): void {
    this.root.classList.add('tf')

    const header = el('header', 'tf-header')
    const brand = el('div', 'tf-brand')
    brand.append(el('span', 'tf-dot'), el('span', undefined, 'term-fleet'))
    this.banner = el('div', 'tf-banner')
    const actions = el('div', 'tf-header-actions')
    const newBtn = el('button', 'tf-btn tf-btn-primary', `新建终端  ${MOD_LABEL}T`)
    newBtn.onclick = () => this.newTab()
    actions.append(newBtn)
    header.append(brand, this.banner, actions)

    const body = el('div', 'tf-body')

    const aside = el('aside', 'tf-side')
    const sideHead = el('div', 'tf-side-head')
    sideHead.append(el('span', undefined, '主机会话'), (this.fleetCount = el('span', 'tf-count', '0')))
    this.fleetList = el('div', 'tf-side-list')
    aside.append(sideHead, this.fleetList)

    const main = el('main', 'tf-main')
    this.tabStrip = el('div', 'tf-tabs')
    this.panes = el('div', 'tf-panes')
    main.append(this.tabStrip, this.panes)

    body.append(aside, main)

    const footer = el('footer', 'tf-status')
    this.statusConn = el('span', 'tf-chip')
    this.statusHost = el('span', 'tf-status-item')
    this.statusSession = el('span', 'tf-status-item')
    footer.append(this.statusConn, this.statusHost, this.statusSession)

    this.toastBox = el('div', 'tf-toasts')
    this.root.append(header, body, footer, this.toastBox)

    this.observer = new ResizeObserver(() => this.scheduleFit())
    this.observer.observe(this.panes)
  }

  private renderFleet(): void {
    this.fleetCount.textContent = String(this.sessions.length)
    this.fleetList.replaceChildren()
    if (!this.sessions.length) {
      this.fleetList.append(el('div', 'tf-empty', '主机上还没有会话'))
      return
    }
    for (const s of this.sessions) {
      const row = el('div', 'tf-session')
      row.dataset.sessionId = s.sessionId
      const open = this.tabs.some((t) => t.sessionId === s.sessionId)
      if (open) row.classList.add('open')
      if (!s.alive) row.classList.add('dead')

      const line1 = el('div', 'tf-session-title')
      line1.append(el('span', 'tf-dot'), el('span', 'tf-session-name', s.title || basename(s.cwd) || s.sessionId))

      const line2 = el('div', 'tf-session-meta')
      const bits = [s.sessionId, `${s.cols}×${s.rows}`, `pid ${s.pid}`]
      if (s.controller) bits.push(`控制中 ${s.controller}`)
      if (!s.alive) bits.push('已退出')
      line2.textContent = bits.join(' · ')

      if (s.alive) {
        const kill = el('button', 'tf-kill', '结束')
        kill.title = '结束该会话（终止主机上的进程）'
        kill.onclick = (ev) => {
          ev.stopPropagation()
          this.killSession(s)
        }
        row.append(kill)
      }

      row.append(line1, line2)
      row.onclick = () => this.attachOrFocus(s.sessionId)
      this.fleetList.append(row)
    }
  }

  private renderTabs(): void {
    this.tabStrip.replaceChildren()
    for (const tab of this.tabs) {
      const btn = el('button', 'tf-tab')
      if (tab.key === this.activeKey) btn.classList.add('active')
      if (!tab.alive && tab.sessionId) btn.classList.add('dead')
      if (tab.role === ROLE.controller) btn.classList.add('owned')

      const dot = el('span', 'tf-tab-dot')
      const label = el('span', 'tf-tab-label', tab.title || '新终端')
      const close = el('span', 'tf-tab-close', '×')
      close.title = '关闭标签（仅停止观看，不结束进程）'
      close.onclick = (ev) => {
        ev.stopPropagation()
        this.closeTab(tab)
      }
      btn.append(dot, label, close)
      btn.onclick = () => this.select(tab.key)
      this.tabStrip.append(btn)
    }
    const add = el('button', 'tf-tab-add', '+')
    add.title = `新建终端 (${MOD_LABEL}T)`
    add.onclick = () => this.newTab()
    this.tabStrip.append(add)
  }

  private renderStatus(): void {
    const label =
      this.connState === 'online' ? '已连接' : this.connState === 'connecting' ? '连接中…' : '已断开'
    this.statusConn.textContent = label
    this.statusConn.className = `tf-chip tf-chip-${this.connState}`
    this.statusConn.title = this.fleet.endpoint

    if (this.host) {
      this.statusHost.textContent = `${this.host.hostname} · ${this.host.platform}/${this.host.arch} · ${this.sessions.length} 个会话`
    } else {
      this.statusHost.textContent = this.fleet.endpoint
    }

    const tab = this.current()
    if (!tab) {
      this.statusSession.textContent = '未选中会话'
      return
    }
    const role =
      tab.role === ROLE.controller ? '控制中' : tab.role === ROLE.viewer ? '只读' : '—'
    const size = `${tab.bundle.term.cols}×${tab.bundle.term.rows}`
    const id = tab.sessionId || '待创建'
    this.statusSession.textContent = `${id} · ${role} · ${size}`
  }

  private renderBanner(): void {
    const tab = this.current()
    let text = ''
    if (tab?.sessionId && tab.alive && tab.role === ROLE.viewer) {
      text = '只读：该会话正被另一个端控制'
    } else if (tab?.sessionId && !tab.alive) {
      text = '会话已退出，可读取输出后关闭标签'
    }
    this.banner.textContent = text
    this.banner.classList.toggle('on', Boolean(text))
  }

  private toast(text: string, kind: 'info' | 'warn' | 'error' = 'info'): void {
    const node = el('div', `tf-toast tf-toast-${kind}`, text)
    this.toastBox.append(node)
    window.setTimeout(() => node.classList.add('out'), 3600)
    window.setTimeout(() => node.remove(), 4200)
  }

  // ---------------------------------------------------------------- actions

  private current(): Tab | undefined {
    return this.tabs.find((t) => t.key === this.activeKey)
  }

  private openTab(sessionId = ''): Tab {
    const bundle = createTerminal()
    const pane = el('div', 'tf-pane')
    bundle.term.open(pane)
    this.attachClipboard(bundle)
    const tab: Tab = {
      key: this.nextKey++,
      sessionId,
      title: '',
      bundle,
      pane,
      role: null,
      alive: true,
    }
    this.panes.append(pane)
    this.tabs.push(tab)
    this.select(tab.key)
    return tab
  }

  private newTab(): void {
    if (!this.fleet.online) {
      this.toast('尚未连接到 daemon', 'warn')
      return
    }
    const tab = this.openTab('')
    const { cols, rows } = this.fitTab(tab)
    tab.title = '新终端'
    // Size is sent with the request so the shell is born at the right width
    // instead of being resized a frame later.
    this.fleet.create({ cols, rows })
    this.renderTabs()
  }

  private attachOrFocus(sessionId: string): void {
    const existing = this.tabs.find((t) => t.sessionId === sessionId)
    if (existing) {
      this.select(existing.key)
      return
    }
    if (!this.fleet.online) {
      this.toast('尚未连接到 daemon', 'warn')
      return
    }
    const tab = this.openTab(sessionId)
    this.fitTab(tab)
    // cursor 0 asks for everything the ring still holds: at most the last
    // 256 KB of output, with `truncated` set if older bytes were dropped. It
    // never replays unbounded history the way attaching from byte 0 would.
    this.fleet.attach(sessionId, 0)
  }

  private select(key: number): void {
    if (this.activeKey === key) return
    this.activeKey = key
    for (const tab of this.tabs) {
      tab.pane.classList.toggle('active', tab.key === key)
    }
    this.renderTabs()
    this.renderBanner()
    this.renderStatus()
    const tab = this.current()
    if (tab) {
      this.fitTab(tab)
      tab.bundle.term.focus()
    }
  }

  private closeTab(tab: Tab): void {
    // Closing a tab means "stop watching". Killing the process is a separate,
    // explicit action because it is irreversible for every other client.
    if (tab.sessionId) this.fleet.detach(tab.sessionId)
    this.removeTab(tab)
  }

  private removeTab(tab: Tab): void {
    tab.bundle.dispose()
    tab.pane.remove()
    this.tabs = this.tabs.filter((t) => t !== tab)
    if (this.activeKey === tab.key) {
      this.activeKey = this.tabs.length ? this.tabs[this.tabs.length - 1].key : -1
    }
    for (const t of this.tabs) t.pane.classList.toggle('active', t.key === this.activeKey)
    this.renderTabs()
    this.renderBanner()
    this.renderStatus()
    const next = this.current()
    if (next) this.fitTab(next)
  }

  private killSession(session: Summary): void {
    const name = session.title || basename(session.cwd) || session.sessionId
    const owner = session.controller ? `当前由 ${session.controller} 控制` : '当前无人控制'
    if (!window.confirm(`结束会话「${name}」？\n\n这会终止主机上的进程，对所有客户端不可见。\n${owner}`)) {
      return
    }
    this.fleet.closeSession(session.sessionId)
  }

  private fitTab(tab: Tab): { cols: number; rows: number } {
    try {
      tab.bundle.fit.fit()
    } catch {
      /* the pane may not be laid out yet */
    }
    const { cols, rows } = tab.bundle.term
    if (tab.sessionId && tab.role === ROLE.controller) {
      this.fleet.resize(tab.sessionId, cols, rows)
    }
    return { cols, rows }
  }

  private scheduleFit(): void {
    if (this.fitTimer !== undefined) clearTimeout(this.fitTimer)
    this.fitTimer = window.setTimeout(() => {
      const tab = this.current()
      if (tab) {
        this.fitTab(tab)
        this.renderStatus()
      }
    }, 60)
  }

  private attachClipboard(bundle: TerminalBundle): void {
    const { term } = bundle
    term.attachCustomKeyEventHandler((ev) => {
      if (ev.type !== 'keydown') return true
      const key = ev.key.toLowerCase()
      const copyCombo = (ev.metaKey && key === 'c') || (ev.ctrlKey && ev.shiftKey && key === 'c')
      if (copyCombo) {
        if (!term.hasSelection()) return true // plain Ctrl+C must still reach the shell
        void copyText(term.getSelection()).then((ok) => {
          if (ok) this.toast('已复制', 'info')
        })
        return false
      }
      return true
    })
    term.onTitleChange((title) => {
      const tab = this.tabs.find((t) => t.bundle === bundle)
      if (!tab || !title) return
      tab.title = title
      this.renderTabs()
    })
  }

  private bindKeys(): void {
    document.addEventListener(
      'keydown',
      (ev) => {
        const mod = ev.metaKey || ev.ctrlKey
        if (!mod) return
        const key = ev.key

        if (key === 't' && !ev.shiftKey) {
          ev.preventDefault()
          this.newTab()
          return
        }
        if (key === 'w' && !ev.shiftKey) {
          ev.preventDefault()
          const tab = this.current()
          if (tab) this.closeTab(tab)
          return
        }
        if (key === 'Tab') {
          ev.preventDefault()
          if (!this.tabs.length) return
          const idx = this.tabs.findIndex((t) => t.key === this.activeKey)
          const next = (idx + (ev.shiftKey ? -1 : 1) + this.tabs.length) % this.tabs.length
          this.select(this.tabs[next].key)
          return
        }
        if (/^[1-9]$/.test(key)) {
          ev.preventDefault()
          const n = Number(key)
          const tab = n === 9 ? this.tabs[this.tabs.length - 1] : this.tabs[n - 1]
          if (tab) this.select(tab.key)
        }
      },
      { capture: true },
    )
  }
}