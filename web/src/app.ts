// The whole client. Deliberately framework-free: the same bundle is what the
// desktop shell and the phone load, so every kilobyte is paid three times.
//
// M3 turns this from "a client for one daemon" into "a client for a fleet":
// several machines are reachable at once, each with its own sessions. There is
// no tab strip: the sidebar is the switcher, one pane is on screen at a time,
// and the pane wears a card's header the way the desktop window does. A pane still
// remembers which machine it belongs to, because one session id is only unique
// within its own host.

import { Fleet, type ConnState, type FleetHandlers } from './fleet'
import { DirectConnector, type Connector } from './link'
import { ERR, ROLE, type Host, type Role, type Summary } from './protocol'
import { RelayConnection, relayURLFromLocation, type RelayState } from './relay'
import { applyTerminalTheme, createTerminal, type TerminalBundle } from './term'
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
  /** The code the shell exited with, for the tab's tooltip. */
  exit?: number
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
  /** Flips <html data-theme> and remembers the choice. */
  onToggleTheme?: () => void
}

const MOD_LABEL = navigator.platform.toLowerCase().includes('mac') ? '⌘' : 'Ctrl'

/**
 * The machine serving this page. Its shells are the ones this client can open
 * without a relay, and the ones nobody else can be handed: see the release
 * button in `renderBanner`.
 */
const LOCAL_MACHINE = 'local'

/** Shells are what a pane runs when it is not running a program. */
const SHELLS = new Set([
  'zsh', 'bash', 'sh', 'dash', 'ksh', 'fish', 'tcsh', 'csh', 'nu', 'elvish',
  'xonsh', 'pwsh', 'powershell', 'cmd', 'cmd.exe',
])

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

/** The short relative time the row shows at the end of a row: now, 4m, 2h, 3d, 1w. */
function ago(ms: number, now = Date.now()): string {
  if (!ms || !Number.isFinite(ms)) return ''
  const d = Math.max(0, now - ms)
  if (d < 60_000) return 'now'
  if (d < 3_600_000) return `${Math.floor(d / 60_000)}m`
  if (d < 86_400_000) return `${Math.floor(d / 3_600_000)}h`
  if (d < 7 * 86_400_000) return `${Math.floor(d / 86_400_000)}d`
  return `${Math.floor(d / (7 * 86_400_000))}w`
}

/** What a waiting session's kind means when the program gave no message. */
function kindLabel(kind?: string): string {
  switch (kind) {
    case 'permission':
      return '需要授权'
    case 'question':
      return '有问题要问'
    case 'auth':
      return '需要登录'
    default:
      return '等你处理'
  }
}

/**
 * the five-state glyph, as markup. The row and the footer wear the same one,
 * at different sizes: a turning arc while a session works, an amber ring with a
 * pulsing dot when it needs a person, a red disc with a bang when it failed, an
 * X ring when it exited, and a check — filled when the finish was unseen, faint
 * once it has been seen.
 */
function statusGlyph(state: string, unread: boolean, size = 15): HTMLElement {
  const box = el('span', 'tf-glyph')
  box.dataset.state = state
  box.dataset.unread = String(unread)
  box.style.width = `${size}px`
  box.style.height = `${size}px`
  const open = (body: string) =>
    `<svg viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">${body}</svg>`
  switch (state) {
    case 'working':
      box.innerHTML = open(
        '<circle cx="8" cy="8" r="6.2" stroke="currentColor" stroke-opacity=".16" stroke-width="1.7"/>' +
          '<circle class="tf-arc" cx="8" cy="8" r="6.2" stroke="currentColor" stroke-width="1.7" stroke-dasharray="9 30" stroke-linecap="round"/>',
      )
      break
    case 'waiting':
      box.innerHTML = open(
        '<circle cx="8" cy="8" r="6.2" stroke="currentColor" stroke-width="1.7"/>' +
          '<circle class="tf-pulse" cx="8" cy="8" r="2.7" fill="currentColor"/>',
      )
      break
    case 'error':
      box.innerHTML = open(
        '<circle cx="8" cy="8" r="7" fill="currentColor"/>' +
          '<path d="M8 4.1v4.7" stroke="var(--bg-raised)" stroke-width="1.8" stroke-linecap="round"/>' +
          '<circle cx="8" cy="11.5" r=".95" fill="var(--bg-raised)"/>',
      )
      break
    case 'exited':
      box.innerHTML = open(
        '<circle cx="8" cy="8" r="6.2" stroke="currentColor" stroke-width="1.5"/>' +
          '<path d="M6.1 6.1l3.8 3.8M9.9 6.1l-3.8 3.8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>',
      )
      break
    default:
      if (unread) {
        box.innerHTML = open(
          '<circle cx="8" cy="8" r="7" fill="currentColor"/>' +
            '<path d="M4.7 8.3l2.2 2.2 4.5-4.9" stroke="var(--bg-raised)" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/>',
        )
      } else {
        box.innerHTML = open(
          '<path d="M3.7 8.3l2.7 2.7 6-6.5" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/>',
        )
      }
  }
  return box
}

/** The small icon buttons the chrome is made of: one glyph, no label. */
function iconButton(className: string, title: string, svg: string, run: (ev: MouseEvent) => void): HTMLButtonElement {
  const btn = el('button', className) as HTMLButtonElement
  btn.type = 'button'
  btn.title = title
  btn.setAttribute('aria-label', title)
  btn.innerHTML = svg
  btn.onclick = run
  return btn
}

const ICON = {
  plus: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>',
  refresh:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/></svg>',
  search:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="11" cy="11" r="7"/><path d="M20 20l-3.5-3.5"/></svg>',
  settings:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.87l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06A1.7 1.7 0 0 0 15 19.4a1.7 1.7 0 0 0-1 1.55V21a2 2 0 1 1-4 0v-.1A1.7 1.7 0 0 0 9 19.4a1.7 1.7 0 0 0-1.87.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-1.55-1H3a2 2 0 1 1 0-4h.1A1.7 1.7 0 0 0 4.6 9a1.7 1.7 0 0 0-.34-1.87l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-1.55V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.55 1.7 1.7 0 0 0 1.87-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.7 1.7 0 0 0 19.4 9a1.7 1.7 0 0 0 1.55 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/></svg>',
  close:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>',
  chevron:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9l6 6 6-6"/></svg>',
  release:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="M16 17l5-5-5-5"/><path d="M21 12H9"/></svg>',
  inspect:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/></svg>',
  theme:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>',
} as const

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
  /**
   * Sessions this client has just let go on purpose, by sessionId. The hub
   * answers a release by handing the lease to the machine the shell runs on,
   * so the releasing client gets a viewer role naming a controller — its own
   * machine. Without this it would toast "已被接管" for something it asked for.
   */
  private letGo = new Set<string>()
  private nextKey = 1

  private root: HTMLElement
  private fleetList!: HTMLElement
  private sideCount!: HTMLElement
  private panes!: HTMLElement
  private composer!: HTMLTextAreaElement
  private sendBtn!: HTMLButtonElement
  private keyHost!: HTMLElement
  private statusConn!: HTMLElement
  private statusHost!: HTMLElement
  private statusSession!: HTMLElement
  private toastBox!: HTMLElement
  private banner!: HTMLElement
  private cardName!: HTMLElement
  private cardDir!: HTMLElement
  private cardChip!: HTMLElement
  private cardRole!: HTMLElement
  private cardDot!: HTMLElement
  private cardClose!: HTMLButtonElement
  private footStats!: HTMLElement
  private filterBar!: HTMLElement
  private filterInput!: HTMLInputElement
  private connectBox?: HTMLElement
  private sideToggle!: HTMLButtonElement
  private hero?: HTMLElement
  private histBar!: HTMLElement
  private histLabel!: HTMLElement
  private histList!: HTMLElement
  private inspectRole!: HTMLElement
  private inspectFields!: HTMLElement
  private inspectToggle?: HTMLButtonElement
  /** The header's 释放控制权, shown only while this client holds the lease. */
  private releaseBtn!: HTMLButtonElement
  /** Groups the person folded up, by machine id. */
  private collapsed = new Set<string>()
  /** What the sidebar's little search field is filtering on. */
  private filter = ''
  private settingsPop?: HTMLElement
  /**
   * Whether the inspector is open. It is the person's choice, so a rerender
   * never overrides it — and it starts closed, as the desktop window's does: a
   * 280px column over a terminal is a terminal cut to 900px, and the button in
   * the header is one click from having it back.
   */
  private inspectOpen = false

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
    this.renderHero()
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
    this.addMachine(LOCAL_MACHINE, connector)
    this.activeMachine = LOCAL_MACHINE
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
    this.renderHero()
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
    this.renderHero()
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

  /**
   * A machine's name as the chrome says it. Before the hello frame arrives a
   * direct connection only knows its socket URL, which is not a machine's name;
   * the window's own daemon is 本机 until it says otherwise.
   */
  private machineTitle(mv: MachineView): string {
    const label = mv.label || mv.id
    if (mv.id === LOCAL_MACHINE && /^wss?:\/\//i.test(label)) return '本机'
    return label
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
        this.renderHero()
        // The status line names this machine's session count, so it is a
        // second reader of the list that just changed: a session created or
        // ended left the line saying 0 个会话 under a sidebar saying one.
        this.renderStatus()
        // Reflect host-side truth: an exit we missed, or someone else's control.
        // A tab that has already been told its shell exited is never revived
        // here — the hub broadcasts the roster right after the exit frame, and
        // the session may still be in it with a stale alive flag, which would
        // put a live-looking tab on screen over a dead shell.
        for (const tab of this.tabs) {
          if (tab.machineId !== machineId || !tab.sessionId) continue
          if (!tab.alive) continue
          const summary = list.find((s) => s.sessionId === tab.sessionId)
          if (!summary) continue
          tab.alive = summary.alive
          // The hub owns the title — a rename anywhere, or a clear, is what
          // this says — so the card reads it rather than keeping its own copy.
          tab.title = summary.title
        }
        this.renderCard()
        this.renderFleet()
        this.renderBanner()
        this.renderInspect()
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
        this.renderCard()
        this.renderFleet()
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
        this.renderCard()
        this.renderFleet()
        this.renderBanner()
        this.renderStatus()
        this.renderInspect()
        this.syncComposer()
        if (role === ROLE.controller) this.composer.focus()
        else if (controller && !this.letGo.delete(sessionId)) {
          this.toast(`已被接管（${controller}），你现在是只读`, 'info')
        }
      },
      exited: (sessionId, code) => {
        const tab = this.findTab(machineId, sessionId)
        if (!tab) return
        tab.alive = false
        // The exit code goes to the banner, not to the card's name: a name
        // with " — 已退出" appended is cut by the ellipsis from the end, which
        // takes the state word first. The state is on the glyph, the dimming
        // and the banner instead.
        tab.exit = code
        tab.bundle.term.write(`\r\n\x1b[2m[进程已退出，退出码 ${code}]\x1b[0m\r\n`)
        this.renderCard()
        this.renderFleet()
        this.renderBanner()
        this.renderInspect()
        this.syncComposer()
      },
      // The hub no longer sends this: a close produces the same exit frame for
      // every watcher, including whoever asked. Kept as a no-op guard so a frame
      // from an older daemon cannot take the pane's final output away.
      closed: (sessionId) => {
        const tab = this.findTab(machineId, sessionId)
        if (tab) tab.alive = false
      },
      errored: (code, message, sessionId) => {
        if (code === ERR.notController) {
          // What the hub refused and why are not always "somebody else has it":
          // a lease that was released and never claimed is free, and the way in
          // is a claim rather than a takeover. Saying "another client controls
          // this" about a shell nobody holds is a sentence that sends a person
          // looking for a person who is not there.
          const holder = this.machine(machineId)?.sessions.find(
            (s) => s.sessionId === sessionId,
          )?.controller
          this.toast(
            holder ? `该会话由 ${holder} 控制，操作已被拒绝` : '该会话当前无人控制：先取得控制权，再试一次',
            'warn',
          )
          const tab = sessionId ? this.findTab(machineId, sessionId) : undefined
          if (tab) {
            tab.role = ROLE.viewer
            this.renderBanner()
            this.renderCard()
            this.renderFleet()
            this.renderInspect()
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
          this.renderCard()
          this.renderFleet()
          this.renderInspect()
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
    this.renderCard()
    this.renderFleet()
    this.renderBanner()
    this.renderInspect()
    this.renderHistory()
    this.renderHero()
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
    this.renderCard()
    this.renderFleet()
  }

  private fleetOf(tab?: Tab): Fleet | undefined {
    if (tab) return this.machine(tab.machineId)?.fleet
    return this.machine(this.activeMachine)?.fleet
  }

  // -------------------------------------------------------------------- DOM

  private build(): void {
    this.root.classList.add('tf')

    const body = el('div', 'tf-body')

    // ---------------------------------------------------------------- sidebar
    const aside = el('aside', 'tf-side')

    const sideTop = el('div', 'tf-side-top')
    const brand = el('div', 'tf-brand')
    brand.append(el('span', 'tf-dot'), el('span', undefined, 'Beacon'))
    const newBtn = iconButton('tf-icon-btn tf-side-new', `新建终端 (${MOD_LABEL}T)`, ICON.plus, () =>
      this.newTab(),
    )
    const refreshBtn = iconButton('tf-icon-btn tf-side-refresh', '刷新会话与机器列表', ICON.refresh, () =>
      this.refreshAll(),
    )
    sideTop.append(brand, el('span', 'tf-spacer'), newBtn, refreshBtn)

    const sideHead = el('div', 'tf-side-head')
    this.sideCount = el('span', 'tf-count', '0')
    sideHead.append(el('span', 'tf-side-label', '会话'), this.sideCount)

    this.filterBar = el('div', 'tf-side-filter')
    this.filterBar.hidden = true
    this.filterInput = el('input', 'tf-side-filter-input') as HTMLInputElement
    this.filterInput.type = 'search'
    this.filterInput.placeholder = '筛选会话…'
    this.filterInput.autocomplete = 'off'
    this.filterInput.spellcheck = false
    this.filterInput.oninput = () => {
      this.filter = this.filterInput.value.trim().toLowerCase()
      this.renderFleet()
    }
    this.filterBar.append(this.filterInput)

    this.fleetList = el('div', 'tf-side-list')
    aside.append(sideTop, sideHead, this.filterBar, this.fleetList, this.buildSideFooter())
    // Tapping a row on a phone is the end of the drawer's business.
    aside.addEventListener('click', (ev) => {
      if ((ev.target as HTMLElement).closest('.tf-session, .tf-machine')) {
        this.root.classList.remove('side-open')
      }
    })

    // ------------------------------------------------------------------- main
    const main = el('main', 'tf-main')
    const card = el('section', 'tf-card')
    const head = el('header', 'tf-card-head')

    this.sideToggle = el('button', 'tf-side-toggle', '☰') as HTMLButtonElement
    this.sideToggle.title = '会话列表'
    this.sideToggle.setAttribute('aria-label', '打开会话列表')
    this.sideToggle.onclick = () => this.root.classList.toggle('side-open')

    const identity = el('div', 'tf-card-id')
    this.cardDot = el('span', 'tf-dot tf-card-dot')
    this.cardName = el('span', 'tf-card-name', '未选择会话')
    this.cardDir = el('span', 'tf-card-dir')
    this.cardChip = el('span', 'tf-card-chip')
    this.cardChip.hidden = true
    this.cardRole = el('span', 'tf-card-role')
    identity.append(this.sideToggle, this.cardDot, this.cardName, this.cardDir, this.cardChip, this.cardRole)

    this.banner = el('div', 'tf-banner')
    // 释放控制权 belongs beside the card, not in the banner. Handing a shell
    // back is an action a client that holds one can always take, not something
    // wrong that needs answering — and the banner is the line for the second
    // kind.
    this.releaseBtn = iconButton(
      'tf-icon-btn tf-release-btn',
      '释放控制权（终端继续运行，改为只读观看）',
      ICON.release,
      () => {
        const tab = this.current()
        if (!tab?.sessionId) return
        this.letGo.add(tab.sessionId)
        this.fleetOf(tab)?.release(tab.sessionId)
        this.toast('已释放控制权，终端继续运行', 'info')
      },
    )
    this.releaseBtn.hidden = true

    const actions = el('div', 'tf-card-actions')
    this.cardClose = iconButton('tf-icon-btn tf-card-close', '关闭面板（仅停止观看，不结束进程）', ICON.close, () => {
      const tab = this.current()
      if (tab) this.closeTab(tab)
    })
    this.cardClose.hidden = true
    actions.append(this.cardClose, this.buildInspectToggle())
    head.append(identity, this.banner, this.releaseBtn, actions)
    this.root.addEventListener('click', (ev) => {
      const target = ev.target as HTMLElement
      if (!this.root.classList.contains('side-open')) return
      if (target.closest('.tf-side') || target.closest('.tf-side-toggle')) return
      this.root.classList.remove('side-open')
    })

    this.panes = el('div', 'tf-panes')
    this.buildHero()
    this.histBar = el('div', 'tf-hist')
    this.histBar.hidden = true
    this.histLabel = el('span', 'tf-hist-label', '历史')
    this.histList = el('div', 'tf-hist-list')
    this.histBar.append(this.histLabel, this.histList)
    this.keyHost = el('div', 'tf-keyhost')
    card.append(head, this.panes, this.histBar, this.buildComposer(), this.keyHost)
    main.append(card)

    body.append(aside, main, this.buildInspectPanel())

    const footer = el('footer', 'tf-status')
    this.statusConn = el('span', 'tf-chip')
    this.statusHost = el('span', 'tf-status-item')
    this.statusSession = el('span', 'tf-status-item')
    footer.append(this.statusConn, this.statusHost, this.statusSession)

    // Theme is a per-device preference. The status line keeps a direct switch;
    // the sidebar's gear opens the same choice with room for more.
    if (this.opts.onToggleTheme) {
      const themeBtn = el('button', 'tf-chip tf-chip-theme') as HTMLButtonElement
      themeBtn.type = 'button'
      const sync = () => {
        const light = document.documentElement.dataset.theme === 'light'
        themeBtn.textContent = light ? '深色' : '浅色'
        themeBtn.title = light ? '切换到深色' : '切换到浅色'
      }
      themeBtn.onclick = () => {
        this.opts.onToggleTheme?.()
        sync()
        this.syncTerminalThemes()
        this.scheduleFit()
      }
      sync()
      footer.append(themeBtn)
    }

    this.toastBox = el('div', 'tf-toasts')
    this.root.append(body, footer, this.toastBox)

    // The composer, not the terminal, is where typing happens; the key bar is
    // the only source of Esc/Tab/Ctrl/arrows for every device now.
    mountTouchKeys(this.keyHost, (data) => this.sendKey(data))

    this.observer = new ResizeObserver(() => this.scheduleFit())
    this.observer.observe(this.panes)
    this.renderCard()
  }

  /** The sidebar's foot: search, settings, then what the fleet wants. */
  private buildSideFooter(): HTMLElement {
    const foot = el('footer', 'tf-side-foot')
    const search = iconButton('tf-icon-btn tf-side-search', '筛选会话', ICON.search, () => this.toggleFilter())
    const settings = iconButton('tf-icon-btn tf-side-settings', '设置', ICON.settings, (ev) => {
      ev.stopPropagation()
      this.toggleSettings()
    })
    this.footStats = el('div', 'tf-foot-stats')
    foot.append(search, settings, this.footStats)
    return foot
  }

  /** Open or close the little search field the footer's magnifier owns. */
  private toggleFilter(): void {
    const open = this.filterBar.hidden
    this.filterBar.hidden = !open
    if (open) {
      this.filterInput.focus()
      return
    }
    this.filterInput.value = ''
    this.filter = ''
    this.renderFleet()
  }

  /** The gear's popover: the theme, a refresh, and nothing invented. */
  private toggleSettings(): void {
    if (this.settingsPop) {
      this.closeSettings()
      return
    }
    const pop = el('div', 'tf-pop')
    const item = (label: string, key: string | undefined, run: () => void): HTMLButtonElement => {
      const row = el('button', 'tf-pop-item') as HTMLButtonElement
      row.type = 'button'
      row.append(el('span', undefined, label))
      if (key) row.append(el('kbd', undefined, key))
      row.onclick = () => {
        this.closeSettings()
        run()
      }
      return row
    }
    const light = document.documentElement.dataset.theme === 'light'
    pop.append(
      item(light ? '切换到深色' : '切换到浅色', undefined, () => {
        this.opts.onToggleTheme?.()
        this.syncTerminalThemes()
        this.scheduleFit()
      }),
      item('会话检视栏', undefined, () => this.toggleInspect()),
      item('刷新会话与机器', undefined, () => this.refreshAll()),
    )
    this.root.append(pop)
    this.settingsPop = pop
  }

  private closeSettings(): void {
    this.settingsPop?.remove()
    this.settingsPop = undefined
  }

  private toggleInspect(): void {
    const panel = this.root.querySelector<HTMLElement>('.tf-inspect')
    if (!panel) return
    this.inspectOpen = !this.inspectOpen
    panel.hidden = !this.inspectOpen
    this.inspectToggle?.setAttribute('aria-pressed', String(this.inspectOpen))
    this.scheduleFit()
  }

  private syncTerminalThemes(): void {
    for (const tab of this.tabs) applyTerminalTheme(tab.bundle.term)
  }

  /**
   * Header affordance for the session inspector (desktop only). It draws the
   * desktop window's icon for the same control — the circled i, not a chevron.
   * A chevron says "expand this", and what this opens is a column of facts
   * about the shell on screen; the two clients should not name one control two
   * ways. The open state is a class, so the button keeps one glyph rather than
   * swapping between two.
   */
  private buildInspectToggle(): HTMLElement {
    const btn = iconButton('tf-icon-btn tf-card-inspect', '会话检视栏', ICON.inspect, () =>
      this.toggleInspect(),
    )
    btn.setAttribute('aria-pressed', String(this.inspectOpen))
    this.inspectToggle = btn
    return btn
  }

  /**
   * The right-hand inspector: what the daemon told us about the session in
   * front of us, in one place. Every field here is real data — nothing is
   * derived or invented.
   */
  private buildInspectPanel(): HTMLElement {
    const panel = el('aside', 'tf-inspect')
    panel.hidden = !this.inspectOpen
    const head = el('div', 'tf-inspect-head')
    head.append(el('h2', undefined, '会话检视'))
    const body = el('div', 'tf-inspect-body')
    this.inspectRole = el('div', 'tf-inspect-role')
    this.inspectFields = el('dl', 'tf-inspect-kv')
    body.append(this.inspectRole, this.inspectFields)
    panel.append(head, body)
    return panel
  }

  private renderInspect(): void {
    if (!this.inspectFields) return
    const panel = this.root.querySelector<HTMLElement>('.tf-inspect')
    const tab = this.current()
    const mv = tab ? this.machine(tab.machineId) : undefined
    const s = tab?.sessionId
      ? mv?.sessions.find((x) => x.sessionId === tab.sessionId)
      : undefined

    this.inspectFields.replaceChildren()
    this.inspectRole.replaceChildren()

    if (!tab || !mv) {
      if (panel) panel.hidden = true
      if (this.inspectToggle) this.inspectToggle.hidden = true
      return
    }
    if (panel) panel.hidden = !this.inspectOpen
    if (this.inspectToggle) {
      this.inspectToggle.hidden = false
      this.inspectToggle.setAttribute('aria-pressed', String(this.inspectOpen))
    }

    const role = tab.role === ROLE.controller ? '控制中' : tab.role === ROLE.viewer ? '只读' : '—'
    this.inspectRole.className = `tf-inspect-role role-${
      tab.role === ROLE.controller ? 'ctrl' : tab.role === ROLE.viewer ? 'view' : 'dead'
    }`
    this.inspectRole.textContent = role

    const row = (k: string, v: string, tag?: string): void => {
      const dt = el('dt', undefined, k)
      const dd = el('dd')
      dd.textContent = v
      if (tag) dd.append(el('span', 'tag', tag))
      this.inspectFields.append(dt, dd)
    }

    row('会话', tab.sessionId || '待创建')
    row('cwd', s?.cwd ?? '—')
    row('尺寸', `${tab.bundle.term.cols}×${tab.bundle.term.rows}`)
    row('pid', s ? String(s.pid) : '—')
    row('控制端', s?.controller ?? '—', s?.controller ? undefined : '本端')
    row('机器', this.machineTitle(mv))
    if (this.multi) row('中继', this.opts.ws)
  }

  /** The composer's command history as tappable chips, newest first. */
  private renderHistory(): void {
    const tab = this.current()
    this.histList.replaceChildren()
    if (!tab || !tab.history.length) {
      this.histBar.hidden = true
      return
    }
    this.histBar.hidden = false
    const shown = tab.history.slice(-12).reverse()
    shown.forEach((cmd) => {
      const chip = el('button', 'tf-hist-item')
      chip.append(el('span', 'i', '❯'))
      chip.append(el('span', undefined, cmd))
      chip.title = '填入输入框'
      chip.onclick = () => {
        this.composer.value = cmd
        this.growComposer()
        this.composer.focus()
      }
      this.histList.append(chip)
    })
  }

  /** Every session on every machine, in the order the sidebar draws them. */
  private allSessions(): Array<{ mv: MachineView; s: Summary }> {
    const out: Array<{ mv: MachineView; s: Summary }> = []
    for (const mv of this.orderedMachines()) {
      for (const s of mv.sessions) out.push({ mv, s })
    }
    return out
  }

  private orderedMachines(): MachineView[] {
    return [...this.machines.values()].sort((a, b) => {
      if (a.online !== b.online) return a.online ? -1 : 1
      return (a.label || a.id).localeCompare(b.label || b.id)
    })
  }

  private renderFleet(): void {
    const total = [...this.machines.values()].reduce((n, mv) => n + mv.sessions.length, 0)
    this.sideCount.textContent = String(total)
    this.fleetList.replaceChildren()

    if (!this.machines.size) {
      this.fleetList.append(
        el(
          'div',
          'tf-empty',
          this.multi ? '中继上还没有登记任何机器' : '还没有会话，点 + 新建一个终端。',
        ),
      )
      this.renderFoot()
      return
    }

    for (const mv of this.orderedMachines()) {
      const folded = this.collapsed.has(mv.id)
      const head = el('div', 'tf-machine')
      if (mv.id === this.activeMachine) head.classList.add('active')
      if (!mv.online) head.classList.add('off')
      if (folded) head.classList.add('collapsed')
      head.append(el('span', `tf-dot ${mv.online ? 'on' : 'off'}`))
      head.append(el('span', 'tf-machine-name', this.machineTitle(mv)))
      head.append(el('span', 'tf-machine-count', String(mv.sessions.length)))
      const chev = el('span', 'tf-machine-chev')
      chev.innerHTML = ICON.chevron
      head.append(chev)
      head.title = [
        this.machineTitle(mv),
        mv.online ? '在线' : '离线',
        `${mv.sessions.length} 个会话`,
        mv.host ? `${mv.host.platform}/${mv.host.arch}` : '',
      ]
        .filter(Boolean)
        .join(' · ')
      head.onclick = () => {
        if (this.activeMachine === mv.id) {
          if (folded) this.collapsed.delete(mv.id)
          else this.collapsed.add(mv.id)
        } else {
          this.activeMachine = mv.id
          this.collapsed.delete(mv.id)
        }
        this.renderFleet()
        this.renderStatus()
        this.renderHero()
      }
      this.fleetList.append(head)
      if (!folded) this.renderSessionsInto(this.fleetList, mv)
    }
  }

  /** Whether a row survives the sidebar's little search field. */
  private matchesFilter(s: Summary): boolean {
    if (!this.filter) return true
    return [s.title, s.cwd, s.sessionId, s.shell, s.controller, s.message]
      .filter(Boolean)
      .some((v) => String(v).toLowerCase().includes(this.filter))
  }

  private renderSessionsInto(host: HTMLElement, mv: MachineView): void {
    let shown = 0
    for (const s of mv.sessions) {
      if (!this.matchesFilter(s)) continue
      shown++
      const status = s.status || (!s.alive ? 'exited' : 'idle')
      const unread = Boolean(s.unread)
      const progress = typeof s.progress === 'number' ? s.progress : -1
      const open = this.tabs.find((t) => t.machineId === mv.id && t.sessionId === s.sessionId)
      const mine = open?.role === ROLE.controller
      const when = ago(s.since || Date.parse(s.createdAt))

      const row = el('div', 'tf-session')
      row.dataset.sessionId = s.sessionId
      row.dataset.machine = mv.id
      if (open) row.classList.add('open')
      if (open && open.key === this.activeKey) row.classList.add('active')
      if (!s.alive) row.classList.add('dead')
      if (unread) row.classList.add('unread')
      if (mine) row.classList.add('owned')
      else if (s.controller) row.classList.add('readonly')

      const line = el('div', 'tf-session-title')
      const name = el('span', 'tf-session-name', s.title || basename(s.cwd) || s.sessionId)
      line.append(name)

      // A badge names a state that is not the ordinary one: the two that mean
      // this client cannot type, or that there is nothing left to type into.
      if (!s.alive) {
        line.append(el('span', 'tf-session-badge b-dead', '已退出'))
      } else if (s.controller && !mine) {
        line.append(el('span', 'tf-session-badge b-view', '只读'))
      }

      // What a session wants, in its own row: the message a program gave, the
      // kind of block when it gave none, or how far along it is. The full text
      // is in the tooltip; the row has one line and a name to keep.
      if (status === 'waiting') {
        const note = el('span', 'tf-session-note is-warn', s.message || kindLabel(s.kind))
        note.title = s.message || kindLabel(s.kind)
        line.append(note)
      } else if (status === 'working') {
        const note = progress >= 0 ? `${progress}%` : ago(s.since || 0)
        if (note) {
          const chip = el('span', 'tf-session-note is-quiet', note)
          chip.title = progress >= 0 ? `进度 ${progress}%` : '运行中'
          line.append(chip)
        }
      }

      if (when) line.append(el('span', 'tf-session-when', when))
      line.append(statusGlyph(status, unread))

      // 结束 is the row's one destructive control, and it arrives with the
      // pointer: the slot is reserved whether or not it is drawn, so hovering
      // never moves the glyph beside it.
      const act = el('span', 'tf-session-act')
      if (s.alive) {
        const kill = el('button', 'tf-kill', '✕')
        kill.type = 'button'
        kill.title = '结束该会话（终止主机上的进程）'
        kill.onclick = (ev) => {
          ev.stopPropagation()
          this.killSession(mv, s)
        }
        act.append(kill)
      }
      line.append(act)

      const standing = !s.alive
        ? '已退出'
        : mine
          ? '控制中'
          : s.controller
            ? `只读 ${s.controller}`
            : '无人控制'
      row.title = [
        s.title || basename(s.cwd) || s.sessionId,
        s.cwd,
        s.sessionId,
        `pid ${s.pid} · ${s.cols}×${s.rows}`,
        standing,
        s.message,
      ]
        .filter(Boolean)
        .join('\n')

      row.append(line)
      row.onclick = () => this.attachOrFocus(mv.id, s.sessionId)
      // Every list on a Mac renames what it lists on a double-click. The hub
      // keeps the title, so every client watching the shell reads it.
      row.ondblclick = (ev) => {
        ev.preventDefault()
        this.renameSession(mv, s, name)
      }
      host.append(row)
    }

    if (shown === 0) {
      if (this.filter) {
        host.append(el('div', 'tf-empty tf-nested', '没有匹配的会话'))
      } else if (!mv.sessions.length) {
        host.append(el('div', 'tf-empty tf-nested', '没有会话'))
      }
    }
  }

  /**
   * Rename a shell from its row: the name becomes a field, Enter commits the
   * title to the hub and Escape or a blur lets it go. The name travels as the
   * session's title, so the row and every other client read it too.
   */
  private renameSession(mv: MachineView, s: Summary, label: HTMLElement): void {
    if (!mv.fleet.online) {
      this.toast('尚未连接到任何主机', 'warn')
      return
    }
    if (!label.isConnected || label.querySelector('input')) return
    const input = el('input', 'tf-session-rename') as HTMLInputElement
    input.type = 'text'
    input.value = s.title || basename(s.cwd) || s.sessionId
    input.placeholder = '终端名称'
    input.title = '回车命名，Esc 取消'
    label.replaceWith(input)
    input.focus()
    input.select()

    let done = false
    const finish = (commit: boolean) => {
      if (done) return
      done = true
      const title = input.value.trim()
      input.replaceWith(label)
      if (commit && title && title !== s.title) {
        s.title = title
        mv.fleet.rename(s.sessionId, title)
        this.renderFleet()
        this.renderCard()
      }
    }
    input.onkeydown = (ev) => {
      ev.stopPropagation()
      if (ev.key === 'Enter') {
        ev.preventDefault()
        finish(true)
      } else if (ev.key === 'Escape') {
        ev.preventDefault()
        finish(false)
      }
    }
    input.onblur = () => finish(true)
  }

  /** The card's title row, from the session the pane is showing. */
  private renderCard(): void {
    const tab = this.current()
    const mv = tab ? this.machine(tab.machineId) : undefined
    const s =
      tab?.sessionId && mv ? mv.sessions.find((x) => x.sessionId === tab.sessionId) : undefined

    this.cardDot.className = 'tf-dot tf-card-dot'
    this.cardClose.hidden = !tab
    if (!tab) {
      this.cardName.textContent = '未选择会话'
      this.cardDir.textContent = ''
      this.cardDir.hidden = true
      this.cardChip.hidden = true
      this.cardRole.textContent = ''
      this.cardRole.className = 'tf-card-role'
      this.cardDot.classList.add('role-idle')
      return
    }

    this.cardName.textContent = s?.title || tab.title || basename(s?.cwd || '') || '新终端'
    this.cardDir.textContent = s?.cwd || ''
    this.cardDir.hidden = !s?.cwd
    this.cardDir.title = s?.cwd || ''
    // The program in the pane when one is running: "claude" is worth a chip,
    // "zsh" beside a shell named zsh is a chip saying nothing. On a machine the
    // window reaches through the relay the chip names the machine instead.
    const running = s?.process && !SHELLS.has(s.process.toLowerCase()) ? s.process : ''
    const chip = this.multi
      ? mv ? this.machineTitle(mv) : tab.machineId
      : running || (s?.shell ? basename(s.shell) : '')
    this.cardChip.textContent = chip
    this.cardChip.hidden = !chip
    if (chip) {
      this.cardChip.title = this.multi
        ? `运行在 ${chip}`
        : running
          ? `前台程序 ${chip}`
          : `外壳 ${chip}`
    }

    let role = ''
    let cls = 'role-idle'
    if (!tab.alive) {
      role = '已退出'
      cls = 'role-dead'
    } else if (tab.role === ROLE.controller) {
      role = '控制中'
      cls = 'role-ctrl'
    } else if (tab.role === ROLE.viewer) {
      role = '只读'
      cls = 'role-view'
    }
    this.cardRole.textContent = role
    this.cardRole.className = `tf-card-role ${cls}`
    this.cardDot.classList.add(cls)
  }

  /** The sidebar's foot: what needs a person, what is busy, what finished. */
  private renderFoot(): void {
    const sessions = this.allSessions()
    const count = (kind: 'waiting' | 'working' | 'idle') =>
      sessions.filter(({ s }) => {
        const state = s.status || (s.alive ? 'idle' : 'exited')
        return kind === 'idle' ? state === 'idle' && s.unread : state === kind
      }).length
    const waiting = count('waiting')
    const working = count('working')
    const done = count('idle')

    this.footStats.replaceChildren()
    if (waiting + working + done === 0) {
      // Nothing wants attention: the foot says how the fleet stands, and a
      // click asks the roster again.
      const fleet = el('button', 'tf-foot-fleet') as HTMLButtonElement
      fleet.type = 'button'
      const online = [...this.machines.values()].filter((mv) => mv.online).length
      fleet.append(el('span', `tf-dot ${online ? 'on' : 'off'}`))
      fleet.append(el('span', undefined, String(online)))
      fleet.title = `${online} 台机器在线 · 刷新`
      fleet.onclick = () => this.refreshAll()
      this.footStats.append(fleet)
      return
    }

    const chip = (kind: 'waiting' | 'working' | 'idle', n: number, cls: string, title: string) => {
      if (!n) return
      const btn = el('button', `tf-foot-chip ${cls}`) as HTMLButtonElement
      btn.type = 'button'
      btn.title = title
      btn.append(statusGlyph(kind, kind === 'idle', 12))
      btn.append(el('span', undefined, String(n)))
      btn.onclick = () => this.jumpTo(kind)
      this.footStats.append(btn)
    }
    chip('waiting', waiting, 'is-warn', '需要你：跳到下一个等待的会话')
    chip('working', working, 'is-working', '运行中：跳到下一个正在工作的会话')
    chip('idle', done, 'is-done', '已完成：跳到下一个刚完成的会话')
  }

  /** Walk the sessions that answer to a kind of attention, after the current. */
  private jumpTo(kind: 'waiting' | 'working' | 'idle'): void {
    const entries = this.allSessions()
    if (!entries.length) return
    const active = entries.findIndex(
      ({ mv, s }) =>
        this.current()?.machineId === mv.id && this.current()?.sessionId === s.sessionId,
    )
    const matches = ({ s }: { s: Summary }): boolean => {
      const state = s.status || (s.alive ? 'idle' : 'exited')
      return kind === 'idle' ? state === 'idle' && Boolean(s.unread) : state === kind
    }
    for (let off = 1; off <= entries.length; off++) {
      const i = (active + off + entries.length) % entries.length
      const { mv, s } = entries[i]
      if (!matches({ s })) continue
      this.collapsed.delete(mv.id)
      this.activeMachine = mv.id
      this.attachOrFocus(mv.id, s.sessionId)
      return
    }
    const note =
      kind === 'waiting'
        ? '没有会话需要你'
        : kind === 'working'
          ? '没有会话在运行'
          : '没有刚完成的会话'
    this.toast(note, 'info')
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
      this.statusHost.textContent = `${this.machineTitle(mv)} · ${mv.host.platform}/${mv.host.arch} · ${mv.sessions.length} 个会话`
    } else {
      this.statusHost.textContent = mv ? this.machineTitle(mv) : ''
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

    // A banner action: what the button says, what it does, and whether it is
    // the one that ends something. There is more than one of them now, because
    // 接管, 释放, 关闭面板 and 结束 are four different questions and a person
    // looking at a shell asks them at different moments.
    const actions: Array<{ label: string; title: string; run: () => void; tone?: string }> = []

    const mv = tab ? this.machine(tab.machineId) : undefined
    let text = ''
    if (tab && mv && !mv.online) {
      text = `${this.machineTitle(mv)} 离线，重连后将自动恢复`
    } else if (tab?.sessionId && !tab.alive) {
      // The shell is gone. Its output is still on screen and still worth
      // reading, so the pane stays until it is closed on purpose — 结束 never
      // took the pane away, which is why this is here to be pressed. The
      // sentence does not name the button beside it: a banner that reads
      // "…关闭面板 [关闭面板]" is a banner nobody reads twice.
      text =
        tab.exit !== undefined
          ? `会话已退出（退出码 ${tab.exit}），最后几行仍可读取。`
          : '会话已退出，最后几行仍可读取。'
      actions.push({
        label: '关闭面板',
        title: '关闭面板（仅停止观看）',
        run: () => this.closeTab(tab),
      })
    } else if (tab?.sessionId && tab.alive && tab.role !== ROLE.controller) {
      // The sessions broadcast, not the role frame, is what tells us whether
      // control is merely unclaimed or actually held elsewhere.
      const holder = mv?.sessions.find((s) => s.sessionId === tab.sessionId)?.controller ?? ''
      if (mv?.controlOff) {
        // The machine answered a claim by refusing it: it allows watching and
        // not typing. The button stays — the switch can move back at any time —
        // but nobody should have to guess why their typing is going nowhere.
        text = `只读：${this.machineTitle(mv)} 不允许其他客户端控制它的终端`
      } else {
        text = holder ? `只读：该会话正被 ${holder} 控制` : '只读：当前无人控制'
      }
      // Taking a shell from under someone is a thing a person asks for: an
      // unclaimed shell is claimed outright, a held one is confirmed first,
      // and the holder is told either way.
      actions.push({
        label: '接管',
        title: '接管这个会话（对方会变成只读）',
        run: () => {
          if (holder && !window.confirm(`从 ${holder} 手中接管这个会话？\n\n对方会收到通知并变为只读。`)) return
          this.letGo.delete(tab.sessionId)
          this.fleetOf(tab)?.claim(tab.sessionId, Boolean(holder))
        },
      })
    } else if (tab?.sessionId && tab.alive && tab.role === ROLE.controller) {
      // This client holds the lease: the ordinary state, so the banner stays
      // empty. Letting go is its own act rather than the absence of one — but
      // only where there is somebody to let go *to*, which is the shell this
      // client is driving on another machine: the lease goes back to the
      // machine the shell runs on and nobody is left looking at a mute
      // terminal. On a shell of this machine's the lease would go to nobody and
      // this window would stop taking its own person's typing, so 释放 is not
      // drawn for it. Handing one of those over is the other client's 接管.
    }
    // 释放控制权 is for the shell this window is driving on another machine.
    this.releaseBtn.hidden = !(
      tab?.sessionId &&
      tab.alive &&
      tab.role === ROLE.controller &&
      tab.machineId !== LOCAL_MACHINE
    )

    this.banner.classList.toggle('on', Boolean(text))
    if (!text || !tab) return
    this.banner.append(el('span', undefined, text))
    for (const a of actions) {
      const button = el('button', 'tf-btn tf-btn-small', a.label)
      button.title = a.title
      button.onclick = a.run
      this.banner.append(button)
    }
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
    const brand = el('div', 'tf-connect-brand')
    brand.append(el('span', 'tf-dot'), el('span', undefined, 'Beacon'))
    card.append(brand, el('h1', undefined, '连接中继'), el('p', 'tf-connect-note', reason))

    const url = el('input', 'tf-input') as HTMLInputElement
    url.type = 'text'
    url.value = this.opts.ws
    url.placeholder = 'wss://relay.example.com/relay/client'

    const token = el('input', 'tf-input') as HTMLInputElement
    token.type = 'password'
    token.value = this.opts.token ?? ''
    token.placeholder = '访问令牌'
    token.autocomplete = 'current-password'

    // An empty token is the one mistake a person makes here, so it gets said
    // next to the field rather than by the button doing nothing.
    const err = el('p', 'tf-connect-err', '请输入访问令牌')
    err.hidden = true

    const submit = el('button', 'tf-btn tf-btn-primary', '连接')
    const go = () => {
      const t = token.value.trim()
      if (!t) {
        err.hidden = false
        token.focus()
        return
      }
      err.hidden = true
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
    form.append(err, submit)
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
    // The prompt glyph is the same shape as the shell's, so the composer reads
    // as a continuation of the terminal rather than a form field.
    const prompt = el('span', 'tf-prompt', '❯')
    const input = el('textarea', 'tf-composer-input') as HTMLTextAreaElement
    input.rows = 1
    input.placeholder = '输入命令，回车发送（Shift+回车换行）'
    input.autocomplete = 'off'
    input.spellcheck = false
    // A soft keyboard that "helpfully" capitalizes or autocorrects corrupts
    // shell input; xterm sets the same attributes on its own textarea.
    input.setAttribute('autocapitalize', 'off')
    input.setAttribute('autocorrect', 'off')
    const send = el('button', 'tf-composer-send', '发送') as HTMLButtonElement
    send.append(el('kbd', undefined, `${MOD_LABEL}↵`))
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

    // The prompt and the input are one control, drawn as one field: a focus
    // ring around both is what says "type here", and it is the field that goes
    // quiet when this client has nothing it may type into.
    const field = el('div', 'tf-c-field')
    field.append(prompt, input)
    bar.append(field, send)
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
    this.renderHistory()
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
    // Grows to the CSS max-height and then scrolls: the number here is
    // .tf-composer-input's max-height, so the two cannot disagree.
    this.composer.style.height = 'auto'
    this.composer.style.height = `${Math.min(this.composer.scrollHeight, 120)}px`
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
      this.toast(`${this.machineTitle(mv)} 尚未连接`, 'warn')
      return
    }
    // The new shell opens where the shell on screen stands, so a second tab
    // continues in the directory the first one is in rather than at home. The
    // first tab of a machine has nothing to follow and gets the default. Read
    // before the tab is opened: opening one selects it.
    const onScreen = this.current()
    const from = onScreen && onScreen.machineId === mv.id ? onScreen.sessionId : ''
    const tab = this.openTab(mv.id, '')
    const { cols, rows } = this.fitTab(tab)
    tab.title = '新终端'
    // Size is sent with the request so the shell is born at the right width
    // instead of being resized a frame later.
    mv.fleet.create({ cols, rows, from })
    this.renderCard()
    this.renderFleet()
  }

  private refreshAll(): void {
    if (this.relay) {
      this.relay.refreshMachines()
    }
    for (const mv of this.machines.values()) {
      if (mv.fleet.online) {
        mv.fleet.list()
      } else {
        mv.fleet.ensureConnected()
      }
    }
    this.toast('已刷新会话与机器列表', 'info')
  }

  private attachOrFocus(machineId: string, sessionId: string): void {
    const existing = this.findTab(machineId, sessionId)
    if (existing) {
      this.select(existing.key)
      return
    }
    const mv = this.machine(machineId)
    if (!mv?.fleet.online) {
      this.toast(`${mv ? this.machineTitle(mv) : machineId} 尚未连接`, 'warn')
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
    this.renderCard()
    this.renderFleet()
    this.renderBanner()
    this.renderStatus()
    this.renderInspect()
    this.renderHistory()
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
    this.letGo.delete(tab.sessionId)
    tab.unwire()
    tab.bundle.dispose()
    tab.pane.remove()
    this.tabs = this.tabs.filter((t) => t !== tab)
    if (this.activeKey === tab.key) {
      this.activeKey = this.tabs.length ? this.tabs[this.tabs.length - 1].key : -1
    }
    for (const t of this.tabs) t.pane.classList.toggle('active', t.key === this.activeKey)
    this.renderCard()
    this.renderFleet()
    this.renderBanner()
    this.renderStatus()
    this.renderInspect()
    this.renderHistory()
    this.renderHero()
    this.syncComposer()
    const next = this.current()
    if (next) this.fitTab(next)
  }

  /**
   * The no-tabs state. Not a dead end: it says what this machine is, and
   * offers the two things worth doing next.
   */
  private buildHero(): void {
    if (this.hero) return
    const hero = el('div', 'tf-hero')
    const mark = el('div', 'tf-hero-mark')
    mark.innerHTML =
      '<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M4 17l4-10 4 10M5.5 13.5h5"/><path d="M14 7v10"/><path d="M14 17c1.8 0 3.4-1.2 3.4-3.2S15.8 10.6 14 10.6"/><path d="M17 10.6l2.4-1"/></svg>'
    const title = el('h1', undefined, '还没有打开的终端')
    const note = el('p')
    note.textContent =
      '在左侧选择一台机器上的会话，或直接新建一个终端。快捷键可以随时用，不用先记住。'
    const acts = el('div', 'tf-hero-acts')
    const newBtn = el('button', 'tf-btn tf-btn-primary', '新建终端')
    newBtn.append(el('kbd', undefined, `${MOD_LABEL}T`))
    newBtn.onclick = () => this.newTab()
    const keysBtn = el('button', 'tf-btn tf-btn-ghost', '快捷键')
    const keys = el('div', 'tf-hero-keys')
    keys.hidden = true
    keys.append(
      el('div', undefined, '新建终端'),
      el('div', undefined, `${MOD_LABEL}T`),
      el('div', undefined, '关闭面板'),
      el('div', undefined, `${MOD_LABEL}W`),
      el('div', undefined, '切换面板'),
      el('div', undefined, `${MOD_LABEL}Tab`),
      el('div', undefined, '跳到第 N 个'),
      el('div', undefined, `${MOD_LABEL}1…9`),
    )
    keysBtn.onclick = () => {
      keys.hidden = !keys.hidden
    }
    acts.append(newBtn, keysBtn)
    hero.append(mark, title, note, acts, keys)
    this.panes.append(hero)
    this.hero = hero
    this.renderHero()
  }

  private renderHero(): void {
    if (!this.hero) return
    this.hero.hidden = this.tabs.length > 0
    if (this.hero.hidden) return
    const mv = this.machine(this.activeMachine) ?? [...this.machines.values()][0]
    const note = this.hero.querySelector<HTMLElement>('p')
    if (!note) return
    if (!this.machines.size) {
      note.textContent = '中继上还没有登记任何机器。在要分享的机器上运行 tfd，连到同一个中继即可。'
    } else if (mv && !mv.online) {
      note.textContent = `${this.machineTitle(mv)} 当前离线，重连后这里的会话会自动回来。`
    } else if (mv && !mv.sessions.length) {
      note.textContent = `${this.machineTitle(mv)} 上还没有会话。新建一个，或在别的客户端上开一个再来看。`
    } else if (mv) {
      note.textContent = `${this.machineTitle(mv)} 上有 ${mv.sessions.length} 个会话。在左侧选一个，或直接新建一个终端。`
    }
  }

  /**
   * End a session: kill the process on its machine, for every client.
   *
   * This is one of four separate things a client can do to a shell, and the
   * point of keeping them apart is that they answer different questions:
   *
   *   接管       take the lease from whoever holds it
   *   释放       give the lease up, leaving the shell running
   *   结束       kill the process, for every client, irreversibly
   *   关闭面板   stop watching, leaving the process alone
   *
   * Ending needs control, and a client that does not hold the shell does not
   * have it — so it claims first: stealing when someone else holds the lease,
   * and simply claiming when nobody does. That is what "结束" means from a row
   * the user is looking at, and it works because the close that follows carries
   * no epoch: the hub checks that this client is the controller, which is the
   * check that matters, rather than an epoch a claim has just moved.
   *
   * A client that already holds the shell skips the claim. The claim is a means
   * to the end, not the end: the tab is left alone either way, because reading
   * the shell's last output is the reason it is still there.
   */
  private killSession(mv: MachineView, session: Summary): void {
    const name = session.title || basename(session.cwd) || session.sessionId
    const mine = this.tabs.some(
      (t) =>
        t.machineId === mv.id &&
        t.sessionId === session.sessionId &&
        t.role === ROLE.controller,
    )
    const held = Boolean(session.controller) && !mine
    if (
      !window.confirm(
        `在 ${this.machineTitle(mv)} 上结束会话「${name}」？\n\n` +
          `这会终止主机上的进程，对所有客户端都不可再恢复。\n` +
          (held
            ? `需要先取得控制权（会从 ${session.controller} 手中接管）。\n`
            : mine
              ? '控制权已经在你手里。\n'
              : '当前无人控制，会先取得控制权。\n') +
          `\n面板不会被关闭：结束后仍可读取最后的输出，再由你决定是否关闭面板。`,
      )
    ) {
      return
    }
    // The claim comes first in every case but one — and the one is not "nobody
    // holds it", which is what this used to assume. Taking a shell from a live
    // holder asks with `steal`; a shell nobody holds is claimed outright. Either
    // way the close has to arrive at a hub that sees this client as the
    // controller, or it is refused as a viewer's, so the claim is not only for
    // other people's shells: on a released one it was the missing step, and 结束
    // did nothing at all.
    if (!mine) mv.fleet.claim(session.sessionId, held)
    mv.fleet.closeSession(session.sessionId)
  }

  private fitTab(tab: Tab): { cols: number; rows: number } {
    try {
      tab.bundle.fit.fit()
    } catch {
      /* the pane may not be laid out yet */
    }
    const { cols, rows } = tab.bundle.term
    // A shell that has ended is not resized. The pane still lays out — the last
    // output is still on screen and the banner appearing above it moves it — but
    // there is no session on the other end of the frame, and the daemon answers
    // one with no_session, which this client shows as an error toast: a red
    // toast raised by the act of reading a shell's last lines.
    if (tab.sessionId && tab.alive && tab.role === ROLE.controller) {
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
      this.renderCard()
      this.renderFleet()
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
        // Escape puts away whatever small thing is open, in the order a person
        // would close them.
        if (ev.key === 'Escape') {
          if (this.settingsPop) {
            this.closeSettings()
            return
          }
          if (!this.filterBar.hidden) {
            this.toggleFilter()
            return
          }
          this.root.classList.remove('side-open')
          return
        }
        const mod = ev.metaKey || ev.ctrlKey
        if (!mod) return
        const key = ev.key

        if (key === 't' && !ev.shiftKey) {
          ev.preventDefault()
          this.newTab()
          return
        }
        if (key === 'k' && !ev.shiftKey) {
          ev.preventDefault()
          if (this.filterBar.hidden) this.toggleFilter()
          this.filterInput.focus()
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
