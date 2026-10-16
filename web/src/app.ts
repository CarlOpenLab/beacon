// The whole client. Deliberately framework-free: the same bundle is what the
// desktop shell and the phone load, so every kilobyte is paid three times.
//
// M3 turns this from "a client for one daemon" into "a client for a fleet":
// several machines are reachable at once, each with its own sessions, and the
// tab strip mixes them. A tab remembers which machine it belongs to, because
// one session id is only unique within its own host.

import { Fleet, type ConnState, type FleetHandlers } from './fleet'
import { DirectConnector, type Connector } from './link'
import { ERR, ROLE, type Host, type Role, type Summary } from './protocol'
import { RelayConnection, relayURLFromLocation, type RelayState } from './relay'
import { createTerminal, type TerminalBundle } from './term'
import { mountTouchKeys } from './touchkeys'
import type { TunnelMachine } from './tunnel'

/** A tab may exist before the server has told us its session id. */
interface Tab {
  key: number
  machineId: string
  sessionId: string
  title: string
  bundle: TerminalBundle
  pane: HTMLElement
  role: Role | null
  alive: boolean
  /** Composer contents kept while another tab is in front. */
  draft: string
  /** Commands this tab has sent, newest last, for ArrowUp recall. */
  history: string[]
  /** Recall position: `history.length` means "back to the live draft". */
  histIdx: number
  /** Undoes the selection wiring, so a closed tab leaves nothing behind. */
  unwire: () => void
}

/** One reachable machine and everything we know about it. */
interface MachineView {
  id: string
  label: string
  /** Roster presence in relay mode; link state in direct mode. */
  online: boolean
  fleet: Fleet
  sessions: Summary[]
  host?: Host
  conn: ConnState
  /**
   * The machine answered a claim with control_off: it lets other clients watch
   * and not type. A lease granted afterwards clears it, because the machine can
   * turn that back on whenever it likes.
   */
  controlOff?: boolean
}

export interface AppOptions {
  mode: 'direct' | 'relay'
  /** WebSocket endpoint: the daemon's /ws, or the relay's /relay/client. */
  ws: string
  /** Relay only. */
  token?: string
  onToken?: (token: string) => void
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
  private opts: AppOptions
  private relay?: RelayConnection

  /** Relay mode: the roster. Direct mode: exactly one synthetic machine. */
  private machines = new Map<string, MachineView>()
  private activeMachine = ''
  private tabs: Tab[] = []
  private activeKey = -1
  private nextKey = 1

  private root: HTMLElement
  private fleetList!: HTMLElement
  private fleetCount!: HTMLElement
  private tabStrip!: HTMLElement
  private panes!: HTMLElement
  private composer!: HTMLTextAreaElement
  private sendBtn!: HTMLButtonElement
  private keyHost!: HTMLElement
  private statusConn!: HTMLElement
  private statusHost!: HTMLElement
  private statusSession!: HTMLElement
  private toastBox!: HTMLElement
  private banner!: HTMLElement
  private connectBox?: HTMLElement
  private sideToggle!: HTMLButtonElement

  private observer?: ResizeObserver
  private fitTimer?: number

  constructor(container: HTMLElement, opts: AppOptions) {
    this.root = container
    this.opts = opts
    this.build()
    this.bindKeys()

    if (opts.mode === 'relay') {
      if (!opts.token) {
        this.showConnect('需要访问令牌才能连接中继')
        return
      }
      this.startRelay(opts.token)
    } else {
      this.startDirect()
    }
  }

  start(): void {
    for (const mv of this.machines.values()) mv.fleet.ensureConnected()
    this.relay?.connect()
    this.renderFleet()
    this.renderStatus()
  }

  /** Reconnect after the user supplies or corrects a token. */
  retryWithToken(token: string): void {
    this.opts = { ...this.opts, token }
    this.opts.onToken?.(token)
    this.hideConnect()
    this.relay?.close()
    this.relay = undefined
    for (const mv of this.machines.values()) mv.fleet.close()
    this.machines.clear()
    this.startRelay(token)
    this.start()
  }

  // ------------------------------------------------------------- transports

  private startDirect(): void {
    const connector = new DirectConnector(this.opts.ws, this.opts.ws)
    this.addMachine('local', connector)
    this.activeMachine = 'local'
  }

  private startRelay(token: string): void {
    this.relay = new RelayConnection({
      url: this.opts.ws,
      token,
      onState: (state, detail) => this.onRelayState(state, detail),
      onMachines: (roster) => this.onRoster(roster),
    })
  }

  private onRelayState(state: RelayState, detail?: string): void {
    if (state === 'offline') {
      this.toast(detail ?? '中继已断开', 'warn')
      // A rejected token is the one failure the user can act on, so ask again
      // rather than retrying forever behind a spinner.
      if (detail?.includes('令牌')) this.showConnect('访问令牌无效，请重新输入')
    }
    this.renderFleet()
    this.renderStatus()
  }

  private onRoster(roster: TunnelMachine[]): void {
    const seen = new Set<string>()
    for (const m of roster) {
      seen.add(m.id)
      const existing = this.machines.get(m.id)
      if (existing) {
        existing.online = m.online
        if (m.name) existing.label = m.name
      } else {
        // A machine we have never seen: give it a Fleet immediately so its
        // sessions appear the moment it comes online. The roster's presence
        // must be carried in, not assumed — a machine can be enrolled but
        // offline, and saying "connected" about it would be a lie the sidebar
        // repeats on every render.
        this.addMachine(m.id, this.relay!.connectorFor(m.id), m.name, m.online)
      }
      // Machines discovered after startup would otherwise never be dialled:
      // their session list stays empty forever and the sidebar is a dead end.
      if (m.online) this.machines.get(m.id)?.fleet.ensureConnected()
    }
    for (const id of [...this.machines.keys()]) {
      if (!seen.has(id)) this.machines.delete(id)
    }
    if (!this.machines.has(this.activeMachine)) {
      this.activeMachine = [...this.machines.keys()][0] ?? ''
    }
    this.renderFleet()
    this.renderStatus()
  }

  private addMachine(id: string, connector: Connector, label?: string, online?: boolean): MachineView {
    const mv: MachineView = {
      id,
      label: label ?? connector.label,
      online: online ?? this.opts.mode === 'direct',
      fleet: new Fleet(this.makeHandlers(id), connector),
      sessions: [],
      conn: 'offline',
    }
    mv.fleet.machineId = id
    this.machines.set(id, mv)
    return mv
  }

  private machine(id: string): MachineView | undefined {
    return this.machines.get(id)
  }

  private get multi(): boolean {
    return this.opts.mode === 'relay'
  }

  // ---------------------------------------------------------------- handlers

  /** Per-machine handler set: every frame is routed by (machine, session). */
  private makeHandlers(machineId: string): FleetHandlers {
    return {
      state: (s, detail) => {
        const mv = this.machine(machineId)
        if (!mv) return
        mv.conn = s
        if (s === 'offline' && detail && !this.multi) this.toast(detail, 'warn')
        // The roster is the authority on presence, so only a machine-level
        // reason is taken as evidence here. A relay outage says nothing about
        // whether a machine is up.
        if (s === 'offline' && detail?.includes('机器离线')) this.markMachineOffline(machineId)
        this.renderFleet()
        this.renderStatus()
        // A link coming back re-sends the role frame; a link going down stops
        // the composer either way, until the app hears otherwise.
        this.syncComposer()
      },
      hello: (host, version) => {
        const mv = this.machine(machineId)
        if (!mv) return
        mv.host = host
        if (host?.hostname && (this.opts.mode === 'direct' || !mv.label || mv.label === machineId)) {
          mv.label = host.hostname
        }
        if (version && version !== 1) {
          this.toast(`服务端协议版本 ${version}，客户端期望 1`, 'warn')
        }
        this.renderFleet()
        this.renderStatus()
        this.syncComposer()
      },
      sessions: (list) => {
        const mv = this.machine(machineId)
        if (!mv) return
        mv.sessions = list
        this.renderFleet()
        // Reflect host-side truth: an exit we missed, or someone else's control.
        for (const tab of this.tabs) {
          if (tab.machineId !== machineId || !tab.sessionId) continue
          const summary = list.find((s) => s.sessionId === tab.sessionId)
          if (!summary) continue
          tab.alive = summary.alive
          if (summary.title) tab.title = summary.title
        }
        this.renderTabs()
        this.renderBanner()
        this.syncComposer()
      },
      created: (sessionId) => {
        const tab = this.pendingTab(machineId)
        if (tab) this.bind(tab, sessionId)
      },
      attached: (sessionId, session, _cursor, truncated) => {
        let tab = this.findTab(machineId, sessionId)
        if (!tab) {
          // Attached without a local create: someone asked for an existing session.
          tab = this.openTab(machineId, sessionId)
        }
        this.bind(tab, sessionId)
        if (!session) return
        tab.alive = session.alive
        tab.title = session.title || basename(session.cwd) || sessionId
        this.renderTabs()
        this.syncComposer()
        if (truncated) {
          // The ring buffer no longer holds everything we missed; the screen we
          // would repaint from is a guess, so start from a clean slate instead.
          tab.bundle.term.reset()
          this.toast(`${tab.title}：断线期间的输出已超出缓冲，已重置画面`, 'warn')
        }
      },
      output: (sessionId, _cursor, data, resync) => {
        const tab = this.findTab(machineId, sessionId)
        if (!tab) return
        if (resync) tab.bundle.term.reset()
        if (data.length) tab.bundle.term.write(data)
      },
      role: (sessionId, role, controller) => {
        const tab = this.findTab(machineId, sessionId)
        if (!tab) return
        tab.role = role
        tab.pane.classList.toggle('readonly', role !== ROLE.controller)
        if (role === ROLE.controller) {
          // The machine lets this client type after all; whatever it answered
          // before is no longer the state of anything.
          const mv = this.machine(machineId)
          if (mv) mv.controlOff = false
        }
        this.renderTabs()
        this.renderBanner()
        this.renderStatus()
        this.syncComposer()
        if (role === ROLE.controller) this.composer.focus()
        else if (controller) {
          this.toast(`已被接管（${controller}），你现在是只读`, 'info')
        }
      },
      exited: (sessionId, code) => {
        const tab = this.findTab(machineId, sessionId)
        if (!tab) return
        tab.alive = false
        tab.title = `${tab.title} — 已退出`
        tab.bundle.term.write(`\r\n\x1b[2m[进程已退出，退出码 ${code}]\x1b[0m\r\n`)
        this.renderTabs()
        this.renderBanner()
        this.syncComposer()
      },
      closed: (sessionId) => {
        const tab = this.findTab(machineId, sessionId)
        if (tab) this.removeTab(tab)
      },
      errored: (code, message, sessionId) => {
        if (code === ERR.notController) {
          this.toast('该会话由其他端控制，输入已被拒绝', 'warn')
          const tab = sessionId ? this.findTab(machineId, sessionId) : undefined
          if (tab) {
            tab.role = ROLE.viewer
            this.renderBanner()
            this.renderTabs()
            this.syncComposer()
          }
          return
        }
        if (code === ERR.leaseHeld) {
          // The lease is live and someone else holds it. The banner offers the
          // steal; a refusal is not an error so much as an answer.
          this.toast(message, 'warn')
          return
        }
        if (code === ERR.controlOff) {
          // The machine allows watching and not typing. The tab stays readable,
          // the banner says why, and the offer to take control stays on screen:
          // the machine can turn that back on, and a button that would work again
          // is better than a button that was deleted.
          const mv = this.machine(machineId)
          if (mv) mv.controlOff = true
          const tab = sessionId ? this.findTab(machineId, sessionId) : undefined
          if (tab) tab.role = ROLE.viewer
          else {
            // A refused create: the tab that was waiting for a shell has nothing
            // to wait for, so it goes rather than sitting there empty.
            const waiting = this.pendingTab(machineId)
            if (waiting) this.removeTab(waiting)
          }
          this.toast(message, 'warn')
          this.renderBanner()
          this.renderTabs()
          this.syncComposer()
          return
        }
        this.toast(`${code}: ${message}`, 'error')
      },
    }
  }

  private markMachineOffline(machineId: string): void {
    const mv = this.machine(machineId)
    if (mv) mv.online = false
    this.renderTabs()
    this.renderBanner()
    this.syncComposer()
  }

  // -------------------------------------------------------------- tab tables

  private findTab(machineId: string, sessionId: string): Tab | undefined {
    return this.tabs.find((t) => t.machineId === machineId && t.sessionId === sessionId)
  }

  /** A tab waiting for its `created` frame, for this machine only. */
  private pendingTab(machineId: string): Tab | undefined {
    return this.tabs.find((t) => t.machineId === machineId && t.sessionId === '')
  }

  private bind(tab: Tab, sessionId: string): void {
    if (tab.sessionId === sessionId) return
    tab.sessionId = sessionId
    tab.title = tab.title || sessionId
    this.renderTabs()
  }

  private fleetOf(tab?: Tab): Fleet | undefined {
    if (tab) return this.machine(tab.machineId)?.fleet
    return this.machine(this.activeMachine)?.fleet
  }

  // -------------------------------------------------------------------- DOM

  private build(): void {
    this.root.classList.add('tf')

    const header = el('header', 'tf-header')
    this.sideToggle = el('button', 'tf-side-toggle', '☰') as HTMLButtonElement
    this.sideToggle.title = '会话列表'
    this.sideToggle.onclick = () => this.root.classList.toggle('side-open')
    const brand = el('div', 'tf-brand')
    brand.append(this.sideToggle, el('span', 'tf-dot'), el('span', undefined, 'term-fleet'))
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
    // Tapping outside the drawer closes it on a phone.
    aside.addEventListener('click', (ev) => {
      if ((ev.target as HTMLElement).closest('.tf-session, .tf-machine')) {
        this.root.classList.remove('side-open')
      }
    })

    const main = el('main', 'tf-main')
    this.tabStrip = el('div', 'tf-tabs')
    this.panes = el('div', 'tf-panes')
    this.keyHost = el('div', 'tf-keyhost')
    main.append(this.tabStrip, this.panes, this.buildComposer(), this.keyHost)

    body.append(aside, main)

    const footer = el('footer', 'tf-status')
    this.statusConn = el('span', 'tf-chip')
    this.statusHost = el('span', 'tf-status-item')
    this.statusSession = el('span', 'tf-status-item')
    footer.append(this.statusConn, this.statusHost, this.statusSession)

    this.toastBox = el('div', 'tf-toasts')
    this.root.append(header, body, footer, this.toastBox)

    // The composer, not the terminal, is where typing happens; the key bar is
    // the only source of Esc/Tab/Ctrl/arrows for every device now.
    mountTouchKeys(this.keyHost, (data) => this.sendKey(data))

    this.observer = new ResizeObserver(() => this.scheduleFit())
    this.observer.observe(this.panes)
  }

  private renderFleet(): void {
    const total = [...this.machines.values()].reduce((n, mv) => n + mv.sessions.length, 0)
    this.fleetCount.textContent = String(total)
    this.fleetList.replaceChildren()

    if (!this.multi) {
      const mv = this.machine('local')
      if (!mv) return
      this.renderSessionsInto(this.fleetList, mv, false)
      return
    }

    if (!this.machines.size) {
      this.fleetList.append(el('div', 'tf-empty', '中继上还没有登记任何机器'))
      return
    }

    const ordered = [...this.machines.values()].sort((a, b) => {
      if (a.online !== b.online) return a.online ? -1 : 1
      return a.label.localeCompare(b.label)
    })

    for (const mv of ordered) {
      const head = el('div', 'tf-machine')
      if (mv.id === this.activeMachine) head.classList.add('active')
      if (!mv.online) head.classList.add('off')
      head.append(el('span', `tf-dot ${mv.online ? 'on' : 'off'}`))
      head.append(el('span', 'tf-machine-name', mv.label || mv.id))
      head.append(el('span', 'tf-machine-state', mv.online ? '' : '离线'))
      head.onclick = () => {
        this.activeMachine = mv.id
        this.renderFleet()
        this.renderStatus()
      }
      this.fleetList.append(head)
      this.renderSessionsInto(this.fleetList, mv, true)
    }
  }

  private renderSessionsInto(host: HTMLElement, mv: MachineView, grouped: boolean): void {
    if (!mv.sessions.length) {
      if (!grouped) host.append(el('div', 'tf-empty', '主机上还没有会话'))
      else if (mv.online) host.append(el('div', 'tf-empty tf-nested', '没有会话'))
      return
    }
    for (const s of mv.sessions) {
      const row = el('div', 'tf-session')
      if (grouped) row.classList.add('tf-nested')
      row.dataset.sessionId = s.sessionId
      row.dataset.machine = mv.id
      const open = this.tabs.some((t) => t.machineId === mv.id && t.sessionId === s.sessionId)
      if (open) row.classList.add('open')
      if (!s.alive) row.classList.add('dead')

      const line1 = el('div', 'tf-session-title')
      line1.append(
        el('span', 'tf-dot'),
        el('span', 'tf-session-name', s.title || basename(s.cwd) || s.sessionId),
      )

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
          this.killSession(mv, s)
        }
        row.append(kill)
      }

      row.append(line1, line2)
      row.onclick = () => this.attachOrFocus(mv.id, s.sessionId)
      host.append(row)
    }
  }

  private renderTabs(): void {
    this.tabStrip.replaceChildren()
    for (const tab of this.tabs) {
      const btn = el('button', 'tf-tab')
      if (tab.key === this.activeKey) btn.classList.add('active')
      if (!tab.alive && tab.sessionId) btn.classList.add('dead')
      if (tab.role === ROLE.controller) btn.classList.add('owned')
      if (!this.machine(tab.machineId)?.online) btn.classList.add('offline')
      btn.dataset.machine = tab.machineId

      const dot = el('span', 'tf-tab-dot')
      const label = el('span', 'tf-tab-label', tab.title || '新终端')
      btn.append(dot)
      if (this.multi) {
        const mv = this.machine(tab.machineId)
        btn.append(el('span', 'tf-tab-machine', mv?.label ?? tab.machineId))
      }
      btn.append(label)
      const close = el('span', 'tf-tab-close', '×')
      close.title = '关闭标签（仅停止观看，不结束进程）'
      close.onclick = (ev) => {
        ev.stopPropagation()
        this.closeTab(tab)
      }
      btn.append(close)

      // Two shells started in the same directory look identical in the strip,
      // so the tooltip carries what distinguishes them.
      const mv = this.machine(tab.machineId)
      const summary = tab.sessionId
        ? mv?.sessions.find((s) => s.sessionId === tab.sessionId)
        : undefined
      btn.title = [
        this.multi ? mv?.label : '',
        tab.sessionId || '尚未创建',
        summary?.cwd,
        summary ? `${summary.cols}×${summary.rows}` : '',
        summary?.controller ? `控制端 ${summary.controller}` : '',
      ]
        .filter(Boolean)
        .join('\n')
      btn.onclick = () => this.select(tab.key)
      this.tabStrip.append(btn)
    }
    const add = el('button', 'tf-tab-add', '+')
    add.title = `新建终端 (${MOD_LABEL}T)`
    add.onclick = () => this.newTab()
    this.tabStrip.append(add)
  }

  private renderStatus(): void {
    let label: string
    let state: ConnState | 'relay'
    if (this.multi && this.relay && this.relay.state !== 'online') {
      state = 'relay'
      label = this.relay.state === 'auth' ? '鉴权中…' : '中继离线'
    } else {
      const tab = this.current()
      const mv = tab ? this.machine(tab.machineId) : this.machine(this.activeMachine)
      const s = mv?.conn ?? 'offline'
      state = s
      label = s === 'online' ? '已连接' : s === 'connecting' ? '连接中…' : '已断开'
      if (s === 'online' && mv && !mv.online) label = '机器离线'
    }
    this.statusConn.textContent = label
    this.statusConn.className = `tf-chip tf-chip-${state}`
    this.statusConn.title = this.relay ? this.opts.ws : (this.machine(this.activeMachine)?.fleet.endpoint ?? '')

    const tab = this.current()
    const mv = tab ? this.machine(tab.machineId) : this.machine(this.activeMachine)
    if (mv?.host) {
      this.statusHost.textContent = `${mv.label} · ${mv.host.platform}/${mv.host.arch} · ${mv.sessions.length} 个会话`
    } else {
      this.statusHost.textContent = mv ? mv.label : ''
    }

    if (!tab) {
      this.statusSession.textContent = '未选中会话'
      return
    }
    const role = tab.role === ROLE.controller ? '控制中' : tab.role === ROLE.viewer ? '只读' : '—'
    const size = `${tab.bundle.term.cols}×${tab.bundle.term.rows}`
    const id = tab.sessionId || '待创建'
    this.statusSession.textContent = `${id} · ${role} · ${size}`
  }

  private renderBanner(): void {
    const tab = this.current()
    this.banner.replaceChildren()

    let text = ''
    let label = ''
    let action: (() => void) | undefined

    const mv = tab ? this.machine(tab.machineId) : undefined
    if (tab && mv && !mv.online) {
      text = `${mv.label} 离线，重连后将自动恢复`
    } else if (tab?.sessionId && !tab.alive) {
      text = '会话已退出，可读取输出后关闭标签'
      label = '关闭标签'
      action = () => this.closeTab(tab)
    } else if (tab?.sessionId && tab.alive && tab.role !== ROLE.controller) {
      // The sessions broadcast, not the role frame, is what tells us whether
      // control is merely unclaimed or actually held elsewhere.
      const holder = mv?.sessions.find((s) => s.sessionId === tab.sessionId)?.controller ?? ''
      if (mv?.controlOff) {
        // The machine answered a claim by refusing it: it allows watching and
        // not typing. The button stays — the switch can move back at any time —
        // but nobody should have to guess why their typing is going nowhere.
        text = `只读：${mv.label} 不允许其他客户端控制它的终端`
      } else {
        text = holder ? `只读：该会话正被 ${holder} 控制` : '只读：当前无人控制'
      }
      label = '接管'
      action = () => {
        // Taking a shell from under someone is a thing a person asks for: an
        // unclaimed shell is claimed outright, a held one is confirmed first,
        // and the holder is told either way.
        if (holder && !window.confirm(`从 ${holder} 手中接管这个会话？\n\n对方会收到通知并变为只读。`)) return
        this.fleetOf(tab)?.claim(tab.sessionId, Boolean(holder))
      }
    }

    this.banner.classList.toggle('on', Boolean(text))
    if (!text || !tab) return
    this.banner.append(el('span', undefined, text))
    if (!action) return
    const button = el('button', 'tf-btn tf-btn-small', label)
    button.onclick = action
    this.banner.append(button)
  }

  private toast(text: string, kind: 'info' | 'warn' | 'error' = 'info'): void {
    const node = el('div', `tf-toast tf-toast-${kind}`, text)
    this.toastBox.append(node)
    window.setTimeout(() => node.classList.add('out'), 3600)
    window.setTimeout(() => node.remove(), 4200)
  }

  // --------------------------------------------------------- relay connect

  private showConnect(reason: string): void {
    if (this.connectBox) return
    const box = el('div', 'tf-connect')
    const card = el('div', 'tf-connect-card')
    card.append(el('h1', undefined, '连接中继'))
    card.append(el('p', 'tf-connect-note', reason))

    const url = el('input', 'tf-input') as HTMLInputElement
    url.type = 'text'
    url.value = this.opts.ws
    url.placeholder = 'wss://relay.example.com/relay/client'

    const token = el('input', 'tf-input') as HTMLInputElement
    token.type = 'password'
    token.value = this.opts.token ?? ''
    token.placeholder = '访问令牌'
    token.autocomplete = 'current-password'

    const submit = el('button', 'tf-btn tf-btn-primary', '连接')
    const go = () => {
      const t = token.value.trim()
      if (!t) {
        token.focus()
        return
      }
      this.opts = { ...this.opts, ws: url.value.trim() || this.opts.ws }
      this.retryWithToken(t)
    }
    submit.onclick = go
    token.onkeydown = (ev) => {
      if (ev.key === 'Enter') go()
    }
    url.onkeydown = (ev) => {
      if (ev.key === 'Enter') token.focus()
    }

    const form = el('div', 'tf-connect-form')
    form.append(el('label', 'tf-label', '中继地址'), url)
    form.append(el('label', 'tf-label', '访问令牌'), token)
    form.append(submit)
    card.append(form)
    card.append(
      el(
        'p',
        'tf-connect-hint',
        '令牌只保存在这台设备的浏览器里，通过首帧发送，不会出现在 URL 或访问日志中。',
      ),
    )
    box.append(card)
    this.root.append(box)
    this.connectBox = box
    window.setTimeout(() => (this.opts.token ? url : token).focus(), 30)
  }

  private hideConnect(): void {
    this.connectBox?.remove()
    this.connectBox = undefined
  }

  // ---------------------------------------------------------------- actions

  private current(): Tab | undefined {
    return this.tabs.find((t) => t.key === this.activeKey)
  }

  /** Injected by the touch key bar; bypasses xterm so escape sequences survive. */
  private sendKey(data: string): void {
    const tab = this.current()
    if (!tab?.sessionId || tab.role !== ROLE.controller || !tab.alive) return
    this.machine(tab.machineId)?.fleet.input(tab.sessionId, data)
  }

  // ---------------------------------------------------------------- composer

  /**
   * The composer is the only keyboard path into a shell. Text is composed
   * locally and sent as one frame on Enter, so the network is paid once per
   * command instead of once per character. Control keys the text box cannot
   * express (Esc, Tab, Ctrl, arrows) come from the key bar below it.
   */
  private buildComposer(): HTMLElement {
    const bar = el('div', 'tf-composer')
    const input = el('textarea', 'tf-composer-input') as HTMLTextAreaElement
    input.rows = 1
    input.placeholder = '输入命令，回车发送（Shift+回车换行）'
    input.autocomplete = 'off'
    input.spellcheck = false
    // A soft keyboard that "helpfully" capitalizes or autocorrects corrupts
    // shell input; xterm sets the same attributes on its own textarea.
    input.setAttribute('autocapitalize', 'off')
    input.setAttribute('autocorrect', 'off')
    const send = el('button', 'tf-btn tf-btn-primary tf-composer-send', '发送') as HTMLButtonElement
    send.type = 'button'

    input.addEventListener('input', () => {
      const tab = this.current()
      if (tab) tab.draft = input.value
      this.growComposer()
    })
    input.addEventListener('keydown', (ev) => {
      if (ev.key === 'Enter' && !ev.shiftKey) {
        ev.preventDefault()
        this.submitComposer()
        return
      }
      if (ev.key !== 'ArrowUp' && ev.key !== 'ArrowDown') return
      // Arrows move the caret in multi-line text; they recall history only
      // while the draft is a single line.
      if (input.value.includes('\n')) return
      ev.preventDefault()
      this.recallHistory(ev.key === 'ArrowUp' ? -1 : 1)
    })
    send.onclick = () => this.submitComposer()

    bar.append(input, send)
    this.composer = input
    this.sendBtn = send
    return bar
  }

  private submitComposer(): void {
    const tab = this.current()
    const text = this.composer.value
    if (!tab?.sessionId || tab.role !== ROLE.controller || !tab.alive) return
    if (!text) return
    tab.draft = ''
    this.composer.value = ''
    this.growComposer()
    tab.history.push(text)
    if (tab.history.length > 100) tab.history.shift()
    tab.histIdx = tab.history.length
    this.machine(tab.machineId)?.fleet.input(tab.sessionId, `${text}\r`)
  }

  /** ArrowUp/ArrowDown through this tab's sent commands; past the newest is the live draft. */
  private recallHistory(dir: number): void {
    const tab = this.current()
    if (!tab?.history.length) return
    tab.histIdx = Math.min(Math.max(tab.histIdx + dir, 0), tab.history.length)
    this.composer.value = tab.histIdx === tab.history.length ? tab.draft : tab.history[tab.histIdx]
    this.growComposer()
  }

  private growComposer(): void {
    this.composer.style.height = 'auto'
    this.composer.style.height = `${Math.min(this.composer.scrollHeight, 132)}px`
  }

  /** Mirror the active tab's state onto the composer: what may be typed, and why not. */
  private syncComposer(): void {
    const tab = this.current()
    const mv = tab ? this.machine(tab.machineId) : undefined
    // A command is an explicit "run this now": it is never queued across a
    // disconnect to fire later unattended. `Fleet`'s buffer still covers the
    // in-flight race — a send that loses the link on the way out — but a
    // composer the user can type into while offline would silently defer.
    const linkUp = mv?.conn === 'online' && mv.online !== false
    const usable =
      !!tab?.sessionId && tab.role === ROLE.controller && tab.alive && linkUp
    this.composer.disabled = !usable
    this.sendBtn.disabled = !usable
    this.keyHost.classList.toggle('off', !usable)
    this.composer.placeholder = usable
      ? '输入命令，回车发送（Shift+回车换行）'
      : !tab?.sessionId
        ? '尚未创建会话'
        : !tab.alive
          ? '会话已退出'
          : tab.role !== ROLE.controller
            ? '只读：等待接管'
            : '连接已断开，重连后即可发送'
  }

  private openTab(machineId: string, sessionId = ''): Tab {
    const bundle = createTerminal()
    const pane = el('div', 'tf-pane')
    bundle.term.open(pane)
    const unwire = this.wireTerminal(bundle, pane)
    const tab: Tab = {
      key: this.nextKey++,
      machineId,
      sessionId,
      title: '',
      bundle,
      pane,
      role: null,
      alive: true,
      draft: '',
      history: [],
      histIdx: 0,
      unwire,
    }
    this.panes.append(pane)
    this.tabs.push(tab)
    this.select(tab.key)
    return tab
  }

  private newTab(): void {
    const mv = this.machine(this.activeMachine) ?? [...this.machines.values()][0]
    if (!mv) {
      this.toast('尚未连接到任何主机', 'warn')
      return
    }
    if (!mv.fleet.online) {
      this.toast(`${mv.label} 尚未连接`, 'warn')
      return
    }
    const tab = this.openTab(mv.id, '')
    const { cols, rows } = this.fitTab(tab)
    tab.title = '新终端'
    // Size is sent with the request so the shell is born at the right width
    // instead of being resized a frame later.
    mv.fleet.create({ cols, rows })
    this.renderTabs()
  }

  private attachOrFocus(machineId: string, sessionId: string): void {
    const existing = this.findTab(machineId, sessionId)
    if (existing) {
      this.select(existing.key)
      return
    }
    const mv = this.machine(machineId)
    if (!mv?.fleet.online) {
      this.toast(`${mv?.label ?? machineId} 尚未连接`, 'warn')
      return
    }
    const tab = this.openTab(machineId, sessionId)
    this.fitTab(tab)
    // cursor 0 asks for everything the ring still holds: at most the last
    // 256 KB of output, with `truncated` set if older bytes were dropped. It
    // never replays unbounded history the way attaching from byte 0 would.
    mv.fleet.attach(sessionId, 0)
  }

  private select(key: number): void {
    if (this.activeKey === key) return
    this.activeKey = key
    for (const tab of this.tabs) {
      tab.pane.classList.toggle('active', tab.key === key)
    }
    const tab = this.current()
    if (tab) this.activeMachine = tab.machineId
    this.renderTabs()
    this.renderFleet()
    this.renderBanner()
    this.renderStatus()
    if (tab) {
      this.fitTab(tab)
      this.composer.value = tab.draft
      // A soft keyboard popping open on every tab switch is hostile; on a
      // touch device the composer takes focus when it is tapped instead.
      if (!matchMedia('(pointer: coarse)').matches) this.composer.focus()
    }
    this.syncComposer()
  }

  private closeTab(tab: Tab): void {
    // Closing a tab means "stop watching". Killing the process is a separate,
    // explicit action because it is irreversible for every other client.
    if (tab.sessionId) this.machine(tab.machineId)?.fleet.detach(tab.sessionId)
    this.removeTab(tab)
  }

  private removeTab(tab: Tab): void {
    tab.unwire()
    tab.bundle.dispose()
    tab.pane.remove()
    this.tabs = this.tabs.filter((t) => t !== tab)
    if (this.activeKey === tab.key) {
      this.activeKey = this.tabs.length ? this.tabs[this.tabs.length - 1].key : -1
    }
    for (const t of this.tabs) t.pane.classList.toggle('active', t.key === this.activeKey)
    this.renderTabs()
    this.renderFleet()
    this.renderBanner()
    this.renderStatus()
    this.syncComposer()
    const next = this.current()
    if (next) this.fitTab(next)
  }

  private killSession(mv: MachineView, session: Summary): void {
    const name = session.title || basename(session.cwd) || session.sessionId
    const owner = session.controller ? `当前由 ${session.controller} 控制` : '当前无人控制'
    if (
      !window.confirm(
        `在 ${mv.label} 上结束会话「${name}」？\n\n这会终止主机上的进程，对所有客户端不可见。\n${owner}`,
      )
    ) {
      return
    }
    mv.fleet.closeSession(session.sessionId)
  }

  private fitTab(tab: Tab): { cols: number; rows: number } {
    try {
      tab.bundle.fit.fit()
    } catch {
      /* the pane may not be laid out yet */
    }
    const { cols, rows } = tab.bundle.term
    if (tab.sessionId && tab.role === ROLE.controller) {
      this.machine(tab.machineId)?.fleet.resize(tab.sessionId, cols, rows)
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

  private wireTerminal(bundle: TerminalBundle, pane: HTMLElement): () => void {
    const { term } = bundle
    // No onData: the terminal's stdin is off. Everything the shell receives
    // arrives through the composer bar or the key bar, which is what keeps
    // typing off the network's critical path.
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
    return this.wireSelection(bundle, pane)
  }

  /**
   * Selection is the one thing the terminal does that the composer cannot, so
   * it gets the affordance a text surface has: drag a range, then decide. The
   * chip appears at the selection when the pointer is up — not mid-drag, where
   * it would jump under the hand making the selection — and copying leaves the
   * selection alone, so a second range can be taken from the same screen.
   *
   * The returned function undoes what was wired, for the tab's close: the
   * pointerup listener is the window's, and a closed tab's terminal is not
   * something a window listener should keep alive.
   */
  private wireSelection(bundle: TerminalBundle, pane: HTMLElement): () => void {
    const { term } = bundle
    const chip = el('button', 'tf-copy-chip', '复制')
    chip.type = 'button'
    chip.hidden = true
    chip.title = '复制选中的文本'
    chip.addEventListener('click', () => {
      const text = term.getSelection()
      if (!text) return
      void copyText(text).then((ok) => {
        if (ok) this.toast('已复制', 'info')
      })
    })
    pane.append(chip)

    let dragging = false
    const onDown = (ev: PointerEvent) => {
      // A press on the chip itself is a copy, not the start of a new range.
      if (chip.contains(ev.target as Node)) return
      dragging = true
      chip.hidden = true
    }
    const onUp = () => {
      dragging = false
      this.placeCopyChip(term, pane, chip)
    }
    const onMenu = (ev: MouseEvent) => {
      // xterm draws its own selection layer, so the browser's Copy — the
      // context menu's, Edit → Copy — has nothing to act on. Right-click
      // copies instead.
      const text = term.getSelection()
      if (!text) return
      ev.preventDefault()
      void copyText(text).then((ok) => {
        if (ok) this.toast('已复制', 'info')
      })
    }
    pane.addEventListener('pointerdown', onDown)
    window.addEventListener('pointerup', onUp)
    pane.addEventListener('contextmenu', onMenu)

    term.onSelectionChange(() => {
      if (dragging) return
      this.placeCopyChip(term, pane, chip)
    })

    return () => {
      pane.removeEventListener('pointerdown', onDown)
      window.removeEventListener('pointerup', onUp)
      pane.removeEventListener('contextmenu', onMenu)
      chip.remove()
    }
  }

  /** Put the chip at the top-left of the selection, clamped inside the pane. */
  private placeCopyChip(term: TerminalBundle['term'], pane: HTMLElement, chip: HTMLElement): void {
    if (!term.hasSelection()) {
      chip.hidden = true
      return
    }
    const first = pane.querySelector('.xterm-selection div')
    if (!first) {
      chip.hidden = true
      return
    }
    const paneBox = pane.getBoundingClientRect()
    const selBox = first.getBoundingClientRect()
    chip.hidden = false
    const chipBox = chip.getBoundingClientRect()
    const top = Math.max(4, selBox.top - paneBox.top - chipBox.height - 4)
    const left = Math.min(
      Math.max(4, selBox.left - paneBox.left),
      Math.max(4, paneBox.width - chipBox.width - 4),
    )
    chip.style.top = `${top}px`
    chip.style.left = `${left}px`
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

export { relayURLFromLocation }
