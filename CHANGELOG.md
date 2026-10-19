# Changelog

Notable changes per release. The release workflow lifts the section matching the
pushed tag straight into the GitHub release notes, so keep the headings exactly
`## <version>`.

## 0.4.14

**Desktop: macOS native dark styling, squircle icon, manual lease control — 桌面端深色外观与交互细节优化**

- **macOS dark styling and clean title bar.** Fix the 1px white line at the top of the
  macOS window by enforcing native dark appearance and using a hidden titlebar
  style. App icon now uses an Apple-compliant squircle to eliminate the white Dock
  background platter.
  — 适配 macOS 原生深色外观，消除顶部白线，规范图标底板避免 Dock 显示白底。
- **Cleaner minimal navigation.** Remove the redundant top-left logo block and
  the intrusive header "新建终端" button. New sessions can be created via the
  sidebar button or `⌘T`. The header supports native window dragging and traffic
  light spacing when the sidebar is collapsed.
  — 精简侧栏顶部标识与顶部新建按钮，支持窗口拖拽及侧栏折叠避让红绿灯。
- **Clear machine status.** Remote online machines now display a green status dot
  instead of grey, preventing confusion about connectivity.
  — 远端在线机器状态指示灯调整为绿色，状态更直观。
- **Human-readable terminal names & recycled numbering.** Terminals now show clean
  names like `zsh 1`, `zsh 2` or `终端 1`, `终端 2` instead of raw internal backend
  IDs (`s1`, `s3`). Closing and opening tabs recycles unused ordinal numbers.
  — 终端名称优化为友好的程序名和序号（如 `zsh 1` 或 `终端 1`），消除裸露的后端内部 ID（`s1`、`s3`），关闭会话后可回收序号。
- **Clearer lease and takeover terminology.** Add self-explanatory tooltips and helper
  descriptions for read-only mode, takeover, and release actions, clarifying multi-client
  keyboard input vs view-only observation.
  — 补充多端协同模式提示，优化只读模式、接管输入及释放控制权的文案与悬浮提示。
- **Explicit, manual lease takeover and release.** Switching to another machine
  no longer automatically claims its shell or drops existing leases. Taking over
  and releasing control are now strictly manual.
  — 切换机器或会话不再自动接管或释放控制权，接管与释放均改为手动操作。

## 0.4.13

**Desktop: a quieter terminal, with its colors intact — 桌面终端界面重构**

- **Neutral black and gray chrome, colorful terminal output.** Remove the violet
  theme accent while preserving the terminal's ANSI palette and TrueColor output.
  The embedded application logo remains visible in the sidebar.
  — 黑灰界面保留彩色终端输出，侧栏显示应用 Logo。
- **More room for the terminal.** Remove the duplicate tab strip, command composer,
  history strip and inspector entry. Type directly in the native Mygo terminal;
  switch sessions in the compact sidebar or with keyboard shortcuts.
  — 移除重复导航和底部命令输入栏，直接在终端内输入。
- **Compact navigation.** Session and machine rows use a single line; long labels
  truncate, and the header and status line stay on one line. `Cmd/Ctrl+B` toggles
  the sidebar and `Cmd/Ctrl+W` closes the current tab.
  — 压缩会话、机器和顶部布局，支持隐藏侧栏及关闭当前标签。
- **Working settings dialog.** Sidebar settings and `Cmd/Ctrl+,` open the same
  Mygo dialog, with Terminal, Remote, Service and Updates sections. Font controls,
  relay credentials and CA, service and control switches, and updates remain
  available. The dialog fits the minimum desktop window size.
  — 设置整合为四分类弹窗，保留字体、远程连接、证书、服务及更新功能。
- **Closed tabs stay closed.** Closing a tab removes its session row without
  ending the process. Local roster refreshes do not reopen dismissed tabs or
  remove remote tabs. Takeover and release behavior is preserved; exited sessions
  keep their final output until explicitly closed.
  — 关闭后不再显示或被刷新重新打开，保留接管、释放及退出前最后输出。

Browser UI remains unchanged in this release. — 本版本仅重构桌面端界面。

## 0.4.12

**The lease's edges: this machine's own shell, a shell nobody holds, a shell that is gone**

- **释放控制权 is not offered on a shell of this machine's.** The button was drawn
  for every shell the window was driving, including the ones running here — and
  on those there is nobody to hand the lease to. The release drops it and the hub
  hands it to no one, so the only thing the button did was stop the window in
  front of the shell from taking the typing of the person sitting at it. It is
  drawn for the shell this window drives *on another machine*, where the lease
  goes back to the machine the shell runs on and that machine can type again. The
  browser client drew it on the same wrong rule and now draws it on the same
  right one.
- **A shell that has ended keeps its tab, and the tab finally knows it.** The hub
  forgets a session the moment its process exits — before the exit frame and
  before the session list — so no summary ever says `alive=false`, and the
  desktop tab, which learned liveness from the attach summary, sat over a dead
  shell offering 结束 for a process that was gone, with no way to close the tab.
  The exit frame is the only place a tab hears it now: 已退出 arrives with its
  exit code in the tooltip, the tab dims with the exit's colour on its dot, its
  row offers 关闭 where a running shell's row offers 结束, and the band offers
  关闭标签 without repeating the sentence. Nothing is sent to a shell that has
  ended — no keystrokes, no resize: a resize answered `no_session` was a red toast
  raised by the act of reading a shell's last lines.
- **The hub's refusals say who holds the shell, or that nobody does.** A frame
  that needs the lease was answered "another client controls s1" whatever the
  state was, including a shell the same client had just let go — a sentence with
  no client to name, which sends the person reading it looking for somebody who is
  not there. The hub now tells a free lease from a lost one: the refusal is
  *nobody holds s1; claim it first* when there is nobody to name, and a frame from
  a client one handoff behind is told the lease has moved rather than that someone
  took it. The browser client's 结束 was the case that mattered: it claimed only
  when somebody else held the lease, so on a released shell the close was refused
  and 结束 did nothing at all. It claims in every case now, stealing only from a
  live holder.
- **A shell nobody holds is marked read-only, in both clients.** A free lease is
  not the same fact as a taken one, but it is the same answer at the keyboard,
  and the desktop drew nothing for it: no mark on the pane, no band, and a lease
  panel that could not be drawn at all. The pane's 只读 chip, the band
  (只读：当前无人控制, with the way in beside it) and the panel (当前无人控制这个终端,
  取得控制权后你就能继续输入。) are drawn from one answer now, on shells that have
  nobody in them as well as shells that have somebody. Rows read 无人控制 where
  they used to read 控制中 for a shell no one is driving, their holder comes from
  the tab's own lease frames rather than from a broadcast that can be a push
  behind, and the tooltip says 控制端 本窗口 for a shell this window drives instead
  of denying it. A lease the window has not heard about yet is marked with
  nothing at all: the word arrives in a role frame, and a window that read that
  silence as 只读 would flash the mark over every shell a person opens.

## 0.4.11

**Direction C, and a light with one meaning**

`designs/ui-redesign/` held three directions and C — Editor Native — is the one
that was picked, so both clients are drawn from it now rather than from the
system they happened to share: one violet accent for what is chosen or in
control, paper-flat surfaces at 6/10/14px, pills instead of underlines, and the
same near-black terminal sheet in both appearances. The one piece of chrome that
is not words — the light along a shell somebody else is typing into — now means
one thing only.

- **Both clients take the C direction.** The accent moves from teal to violet:
  0.4.9 had moved the desktop off its violet onto the browser's teal so the two
  would match, and the direction that was then chosen is violet, so both are
  violet now. Tabs are pills on a raised strip instead of underlines, the
  composer is one bordered field that lights around the caret rather than a card
  with three controls in it, the key bar is one row of equal keys grouped by
  hairlines, and the sidebar, the session inspector and the settings window are
  redrawn on the same tokens. Colour means one thing each — accent for chosen,
  amber for read-only, red for gone, grey for exited — and where the desktop
  drew decoration that carried nothing, it draws nothing.
- **The terminal's sixteen colours are the app's.** The browser client's theme
  and the desktop's carry the same numbers now: the cursor and the ANSI magenta
  are the accent's violet, the selection is that violet at 28%, and the sheet is
  the same `#0d0d12` in both appearances. A shell should look like it belongs to
  the window around it, and ANSI's sixteen are drawn for a dark ground — which is
  why the light appearance keeps a dark terminal rather than inverting one.
- **The light along a taken shell is drawn under one condition, and it reads as
  light.** It used to come on for any shell another client held, including one on
  a machine the person is not sitting at — a light for a process that is
  nobody-here's, which is the ordinary case rather than news. It is drawn only
  for a shell *this* machine runs with somebody else on its keyboard, which is
  the one thing the words on screen do not say from across the room; a shell on
  another machine keeps its 只读 chip, because that is a fact about the pane you
  are looking at. The effect is one family of colours instead of a spectrum (the
  old cut ran violet, pink, coral and peach around the edge — a second and louder
  vocabulary for a fact the pane was already stating in words) and it is a band
  with no line anywhere in it, falling off on both sides of the pane's edge, the
  way the platform's own agent-takeover glow is drawn, rather than a coloured
  border painted on the pane. Where the desktop asks for less motion the colours
  stop travelling and the light stays.
- **The takeover offer says what pressing it does to the other side.** The panel
  under a shell that is not yours names the holder, says that your typing is not
  reaching the shell, and now says what happens to *them*: they go read-only and
  keep watching the terminal afterwards. A button that takes a keyboard away
  from somebody without saying so is one people learn not to press.

**Fixed**

- **释放控制权 was offered by a window that held nothing.** The browser client
  sets `hidden` on the button, and `hidden` is a bare `display: none` from the
  browser's own stylesheet, which loses to any class that sets a display — and
  every control in that client does. The button sat in the header of a window
  with no session at all. The per-class workarounds the stylesheet had
  accumulated (`.tf-hero[hidden]`, `.tf-inspect[hidden]`, …) are one `[hidden]`
  rule now, so the next control that hides itself stays hidden.
- **The browser client's status line went stale** after the session list changed,
  counting sessions the sidebar no longer showed, and the session inspector could
  not be closed — the button that dismissed it reopened it on the next render.
  Both follow the person now: the count follows the roster, and whether the
  inspector is open is a choice that is remembered while the page is.

## 0.4.10

**Four separate acts, and the ones that were missing**

A client watching a shell could take it over but never give it back, and the
button that said 关闭标签页 ended the process instead. Both are fixed, and the
four things a client can do to a shell are now four buttons on both clients.

- **释放控制权 exists.** The release frame was in the protocol and no client
  ever sent it. A client holding the lease is offered 释放 in the header and in
  the banner; pressing it gives the lease up, leaves the process running and
  leaves the tab watching it. The hub hands the lease to the machine the shell
  runs on, so the shell is not left mute.
- **The hub was throwing a release away.** `handleRelease` dropped the lease and
  then asked `giveAway` to hand it to the machine's own window — which is the
  client that had just released it, because `drop` had just added it to the
  watchers. A release from the machine's own window was a no-op, and that
  window is the one that releases most, since it holds its own shells by
  default.
- **The 会话 panel's × closed the tab and ended the shell.** It was wired to the
  same act as 结束, so "close this tab" killed a process on a machine somewhere
  — for every client watching it, not only the window that clicked. It closes
  the tab now, and 结束 has its own button on the row of the shell it ends.
- **A tab whose shell exited is no longer removed.** The tab is where the
  shell's last output is read, so ending a session is not a reason to lose it.
  The browser client already behaved this way; the desktop app's reconcile did
  not.
- **The desktop app catches up with the browser client's layout.** It gains the
  tab strip, the command composer with its history bar, and the 280px session
  inspector, and its sidebar and terminal take the same measurements the
  browser client uses. Without a composer the window had no way to send Esc,
  Tab, Ctrl or an arrow key at all, because a terminal is a display surface and
  cannot produce them.
- **The app icon is transparent.** `resources/icon.png` had an opaque dark plate
  composited behind the logo, which is what put a dark square in the macOS Dock
  and the Windows taskbar. Every icon is rendered from the one transparent
  source now, and a test reads the pixels so it cannot come back.
- **The release pipeline builds the frontend.** The `cli` job never ran
  `build:web`, so every CLI artifact published so far shipped without the
  browser client embedded — `tfd` answered the browser with nothing.
- **The macOS firewall prompt is explained before it appears.** The 本机服务
  switch says that the system asks once about incoming connections and that
  clicking 允许 settles it for good. A service that failed to start is stated in
  the status bar rather than only logged, and the background update check starts
  off instead of calling out on every launch.

## 0.4.9

**One design, two clients**

The browser client and the desktop app are drawn from the same palette now. A
shell in a tab and a shell in the native window say the same things in the same
words, with the same colours, at the same sizes: one accent for what is chosen
or in control, amber for read-only, red for gone, grey for exited.

- The desktop app's palette moves off its violet onto the teal the browser
  client uses, and its terminal takes the same sixteen ANSI colours, so a shell
  belongs to the window around it rather than looking like a program that
  wandered in. The header band is 44px and the status line 30px, the heights the
  browser client uses, and the status line states the connection as a chip — a
  dot in a wash — the way the browser does.
- A shell in the sidebar carries its state as a badge — 控制中, 只读, 已退出 —
  with the holder's name beside it, and the row's tooltip carries the numbers
  the row has no room for. The empty window says what the machine it would open
  a shell on is doing, rather than showing an empty rectangle.
- The browser client is redrawn on the same system: a cold near-black ground
  with hairlines instead of borders, a sidebar whose rows are one badge and one
  number, tabs whose underline is their state, and a right-hand session
  inspector holding what the daemon said about the shell on screen. A dark and a
  light appearance are both complete, and the choice is remembered per device.
- The composer gained a strip of the commands this tab has sent — real history,
  newest first, one tap back into the box — and says on the same line what the
  Enter key will do. The relay's connect screen names the app, says why it is
  asking, and reports an empty token next to the field instead of doing nothing.
- The phone layout is a drawer and a bigger key bar rather than a second design,
  with touch targets at 44px and the terminal kept whole.

**Fixed**

- The browser client's empty state kept a stale line about a previous machine
  after the roster changed, and said nothing at all once the machine had
  sessions to show.

## 0.4.8

**Desktop app**

- Fixed a race on the relay client's roster callback. The window hands the fleet
  link the place its roster goes, and the link's read loop reads it as frames
  arrive: a window opening while the relay was already pushing a roster read and
  wrote the same field from two goroutines. It is guarded now, both where it is
  set and where it is called, so 设置 → 远程 → 应用 on a live link cannot race an
  arriving roster frame.

## 0.4.7

**Desktop app**

- A new terminal opens in this machine's home directory, not wherever the app
  happened to be started. Started by the window server — Finder, the Windows
  shell — that place is the root of the filesystem, so every new shell opened in
  `/`: a directory nobody chose and everybody leaves again. `-cwd` still says
  otherwise, a client that asks for a directory keeps what it asked for, and a
  shell started on another machine opens in *that* machine's home, which is the
  only home that means anything there. The same applies to terminals started from
  a phone or a browser.

## 0.4.6

**Desktop app**

- The lease follows the shell on screen. A window that opened a shell on another
  machine and then moved the screen on — to 本机, or to any other shell — used to
  leave that machine showing 正在被控制 with a keyboard that no longer worked.
  Switching away lets the lease go, the machine whose shell it is takes it back
  the way it does when a holder's lease lapses, and coming back to that shell
  drives it again without a click. The connection to that machine stays: it is
  the lease that moves, not the link.
- Picking a machine in the 机器 panel brings one of its shells on screen when this
  window has one open there, so 切到本机 means the screen as well as the panels:
  two panes and a pane that disagree about what is being worked on is one answer
  too many.
- The hub hands a lease nobody holds — released, let go by a detach, or simply
  lapsed — to the machine's own window when it is watching. A shell of a machine's
  is that machine's to type in, so a client that stops driving it cannot leave it
  mute with nothing on screen to say why.

## 0.4.5

**Desktop app**

- Control belongs to the window that reached the machine. Opening a shell another
  machine was running left this window read-only — a spectator of a shell it had
  just asked to work in — with the glow and 拿回控制权 drawn on the *controlling*
  machine while the machine being driven kept the keyboard. A tab on another
  machine now takes the lease as it opens: this window types, and the machine
  whose shell it is goes read-only behind the glow, saying who is typing into it.
  拿回控制权 there is still one click, and the hub always honours it.
- 会话 lists the picked machine's shells and nothing else. A tab on another
  machine used to be a row in this machine's list — it looked like one of this
  machine's shells — and picking it moved the panels to that machine, so
  switching tabs landed somewhere else entirely.
- The panel under the glow offers 接管 on a shell of another machine's rather than
  拿回控制权, which is what taking one is when the lease was never this window's.

## 0.4.4

**Desktop app**

- Fixed the 会话 panel not switching shells. Clicking a shell another machine was
  running, when this window already had a tab for it, did nothing at all: the
  click was swallowed and the pane stayed where it was, so a machine's shells —
  the tabs opened to watch them — could not be walked between, while this
  machine's own tabs could. The panel now builds one list either way and picking
  a row selects its tab, opening one only when the shell has no tab yet.
- The 会话 panel says which shell is on screen, with the same accent bar the
  picked machine carries, and picking a shell on another machine moves the 机器
  and 会话 panels with it rather than leaving them on a machine that is no longer
  on screen. ⌘⇧] and ⌘⇧[ walk this window's tabs, wherever each shell runs.

## 0.4.3

**Brand**

- The project wears the mascot: a periwinkle planet with a cream face, a ring and
  a terminal prompt on its brow. It replaces the mark cut out of the logo sheet,
  and it is used as drawn rather than recoloured per surface — which is why the
  app icon, the README's logo and the browser favicon are now the same picture on
  the shell's ink. The mascot's face is cream: on a white tile, or on the
  transparent ground a README has on GitHub's light theme, it would run into the
  page. `resources/brand/make-assets.py` rebuilds all three from the master, and
  takes a new render as its argument when the art changes again.

**Desktop app**

- The ring around a shell being driven from elsewhere is quieter: a thinner line,
  a slower flow and a dimmer bloom than the chat-glow example's composer ring,
  which is drawn to be looked at. (The artist's own reference for this state is
  still pending — this is the window's own reading of "a signal, not a show".)

- A machine taking its shell back ends the connection, on the machine that was
  driving it: the tabs it had open there close, the link to that machine is
  dropped, the panels return to this machine, and the status bar says who took
  the shell back. Before this, the driving window was simply left read-only, with
  a tab on screen that could not type and no word about why.
- The panel under the glow — who holds a shell, and 保持只读 / 拿回控制权 — has a
  width of its own and shorter words. It hangs from the top of a pane, and a pane
  can be narrower than a sentence: it used to run past the terminal it was about.
- The 机器 panel is a list you switch between rather than a nest of shells. This
  machine comes first and is picked by default; picking another machine makes it
  the subject of the 会话 panel below and of that panel's 新建标签页, which now
  starts a shell *on that machine* over the relay and opens it here. The relay
  itself has no row any more — it is the pipe, not a machine — and its state
  moved to the line the header and status bar already carry.
- Reaching the other machines no longer needs a secret typed anywhere. 设置 → 远程
  lost the 客户端令牌 field: a window asks its relay for a short-lived *control
  token*, proving itself with the device credential it already joins the fleet
  with, and that is what its client link presents. Picking a machine the window
  cannot reach yet asks first, in a dialog, so "this window is about to ask the
  relay for reach" is something a person sees and can decline. The fleet's shared
  token is unchanged and is now only what a browser or a phone is given:
  `tfrelay -control-token-ttl` (default 15m) bounds what a machine gets, revoking
  a machine at the relay takes its credentials away at once, and the machine's own
  允许被控制 switch is unchanged.

## 0.4.2

**Desktop app**

- Fixed the window mixing up shells from different machines: frames were routed
  to a tab by session id alone, and session ids are counted per machine — every
  machine has an `s1`. A window driving another machine at the same time as it
  held shells of its own could hand one shell's role frame, exit or close to the
  other shell's tab, which moved the lease marks, the glow and the tab itself.
  Frames are matched by the session object now, and a test drives two desktops
  against each other to hold that.
- The glow around a shell another client is driving is MyGo's shader now: the
  ring from `examples/chat-glow`, compiled for the renderer in front of the
  window and interpreted on the CPU where there is none. The stroked-ring
  approximation it replaces is gone, so the previews show the window as it is
  rather than an impression of it. (`github.com/egoist/mygo` v0.4.8 → v0.5.5.)
- The 机器 panel no longer lists this machine twice. The roster is keyed by the
  device id a machine was enrolled under and the window knows itself by hostname,
  so a machine on a relay showed up as 本机 and again as another machine with its
  own shells — which offered to open its own shells over the relay.
- 服务 → 允许被控制 is the machine's answer to "who may drive my shells". Being
  enrolled with a relay is the consent, so the switch starts on; off, other
  clients — a browser, a phone, another machine — watch and nothing more: no
  lease, no new shells, and a holder that is not this machine's window loses the
  shell in the frame the switch moves. The machine's own window is never
  refused. `tfd` gets the same switch as `-allow-control=false`, and a claim that
  is refused says `control_off` rather than being dropped: the browser client
  shows why and keeps its offer to take control, so a machine that turns it back
  on needs no reload.
- 设置 → 远程 has the 客户端令牌 field that the 0.4.0 notes already promised: the
  token had nowhere to be typed, so the window's second link — the one that makes
  the 机器 panel list anything but this machine — could never come up, and the
  panel told people to fill in a control that was not on the form at all. The
  field names itself after the browser's 访问令牌 prompt, its own line says
  whether the link is up or still waiting for 应用, and the panel stops promising
  machines that cannot appear.
- A machine the window cannot reach says why. `openTab` used to fall back to the
  local hub and attach a remote session id to it, which opened a tab that showed
  nothing and explained nothing; it now declines the tab, and the panel carries
  the reason under the machines.
- A shell another client takes over says so at the moment it happens: the panel
  that names the holder and offers 拿回控制权 opens by itself, and so does the one
  a person gets by typing into a shell that is not theirs — the hub already
  answers a refused keystroke loudly, and now the window says it where the typing
  went instead of only in the log. The header carries the same offer, and the
  机器 panel counts the shells on this machine that other clients are driving, so
  "why is my keyboard not working?" has an answer that does not depend on
  noticing a glow on the pane's edge.

## 0.4.1

- Fixed three data races the Linux race detector found in the code that reaches
  another machine: a stream's number and accepted flag were written under the
  client's lock and read under the stream's own (accepting a stream raced the
  window sending on it); a link's writer was set at construction and cleared by
  the link's reader goroutine with nothing between them; and New Tab opened a
  tab — window state — from a goroutine, racing the frame that draws it, which
  on a Go map is a fatal error rather than a stale read. The tab is now opened on
  the window's own thread and the create frame sent from there.

## 0.4.0

**Taking over a shell on another machine** (`docs/takeover.md`)

- A session's control is a **lease**: `{holder, epoch, deadline}`. Attaching
  without it is read-only, every input frame carries the epoch it was sent
  under, and the owner drops frames whose epoch is past — so input from a
  controller that was just revoked cannot land after the handoff. A lease nobody
  renews lapses, and a silent controller cannot lock the machine out of its own
  shell.
- The desktop window is an ordinary hub client rather than a privileged reader
  of its own sessions: its local link is an in-process pipe (`hub.Pipe`), and a
  remote one is a relay stream (`internal/relayclient`). The hub marks the
  window's own client as the *owner*, so the machine's window takes its shell
  back from anyone, with no confirmation — and a remote client's claim against a
  live holder is refused unless it steals, which the browser client confirms
  first and names the holder in.
- The window reaches the rest of the fleet: enrolled with a relay and holding
  its client token, the 机器 panel lists every machine on that relay with the
  shells it is running, and a shell on another machine opens in a tab here under
  that machine's arbitration. The token is the fleet's client secret and today
  the whole authority over every machine on the relay; a per-device ACL is what
  is still missing.
- A shell another client holds glows around its pane — colours cycling through
  the palette, the shape of mygo's chat-glow example — with a popover naming the
  holder and offering 拿回控制权, and the holder's name on the tab's row. The
  glow runs only while someone else holds the lease, is drawn along the pane's
  edge rather than over the text, and its fade and cycle stand still where the
  desktop asks for less motion.
- The hub announces a created session to its creator *before* the session list
  puts it in front of everyone else, so a client's pending tab becomes the tab
  watching the shell instead of a second tab for the same shell. A departing
  controller's lease is released and the epoch bumped in the same frame, so the
  clients still watching learn the shell is free.

**UI**

- The shell window has a left rail: the open shells, `切换到白天/暗黑`, and the
  settings window. The appearance is saved (`theme`: `light`, `dark`, or unset
  for the system) and the terminal follows it; the settings window's 外观 pane
  offers all three modes.
- New mark, taken from the logo sheet's third row, second column: the app icon
  keeps the sheet's dark tile, the README logo and the web favicon keep the white
  tile with dark ink.
- The desktop window has a palette of its own — the navy ground, the violet for
  what is chosen and the green for what is live — instead of the ui's default
  greys, and the terminal is themed in the same palette rather than its own.
- The strip's two controls moved to its bottom, under the shells: the mark heads
  the strip, and what changes the window itself ends it, so the empty middle no
  longer reads as a control that was forgotten. The fleet's state stays under
  them.
- The column beside the strip is two panels with a draggable line between them:
  会话 on top, 机器 under it. The tab controls moved into the 会话 panel's title
  row as icon buttons — they act on that panel — and the window's header is left
  to naming the shell on screen. Until the line is dragged, the machines' panel
  takes the room its content needs and the shells take the rest, whatever height
  the window is.
- The 机器 panel lists the fleet this window can reach: this machine with the
  service's state, the relay it is enrolled with, and every other machine on
  that relay, each with the shells it is running once the window has looked.
- The settings window is a form: every pane's labels sit in a column beside
  their controls, right-aligned as macOS forms do, instead of stacked full-width
  above them. 应用 is offered only when there is something to apply, the panes
  scroll instead of clipping in a short window, and the 更新 pane hides the
  check button on a build that cannot update itself rather than offering a
  control that does nothing. The 远程 pane gained a 客户端令牌 field, which is
  what lets the window drive the rest of the fleet.
- A window flashes the colour of the appearance it is about to paint, so a light
  window no longer opens with a dark frame around it.

**Browser client**

- Selecting text in a terminal now has the affordance a text surface has: drag a
  range and a 复制 button appears at it — the copy is a choice, not something
  that happens because the pointer was released. Right-click copies the
  selection too, since xterm paints selection in its own layer and the
  browser's own Copy has nothing to act on. `⌘C` / `Ctrl+Shift+C` work as
  before.
- Taking a shell from a live holder asks first: the 接管 button confirms, naming
  the holder, and the claim carries `steal`. A refused claim is answered with
  `lease_held` and a line saying who holds it and until when.

## 0.3.0

**Desktop app**

- The desktop app updates itself. It asks the release feed once a day in the
  background and, when a version is out, shows its notes in a window with
  install / remind me later / skip this version; the download is verified
  against the key built into the app before it replaces anything. macOS also
  gets 检查更新… in the app menu, and every platform gets a 更新 pane in
  设置 (automatic checks and downloads, when it last checked, and a check
  now button).
- Updates replace the app where it is installed, so they work for the dmg,
  the Windows installer and the Linux tarball — a .deb is updated by the
  package manager, and the pane says so instead of offering switches that
  would do nothing. Development builds behave the same way.
- Releases now carry, next to the installers, a signed archive and an
  `update-<target>.json` manifest for every desktop platform, plus delta
  updates from the versions before, so an update is usually a fraction of
  the full download.

## 0.2.0

**Clients (web, desktop, phone)**

- Typing moved out of the terminal into a composer bar under it: text is
  composed locally and sent as one frame on Enter, so a slow link is paid
  once per command instead of once per character. The terminal itself is now
  a display surface (stdin off) — selection and copy keep working.
- Control keys a text box cannot express (Esc, Tab, sticky Ctrl/Alt, arrows,
  C-c, C-d) are injected by the accessory key bar, which is now shown on
  every device rather than phones only.
- The composer carries per-tab drafts and ArrowUp recall of sent commands,
  and it locks with the reason on screen when the session is read-only, dead,
  or the link is down — a command is never queued across a disconnect to run
  later unattended.
- Keystrokes that lose the link on the way out are buffered and replayed on
  reconnect instead of vanishing, input bursts coalesce into one frame, and
  the relay path's base64 encoding is linear rather than one concatenation
  per byte, so large pastes stop janking.

**Brand**

- The logo and the web favicon now sit on a white rounded tile with dark
  ink, so the mark stays legible on dark backgrounds; the README no longer
  needs two scheme-specific logo files.

## 0.1.0

First release: terminals from many machines, one browser tab each.

**Relay (`tfrelay`)**

- Routes WebSocket tunnels between machines running `tfd` and browser, phone or
  desktop clients, one client tab per real session on the host.
- Built-in web admin panel at `/admin/`: first run claims an administrator
  account, then machines, credentials, browser tokens and the CA certificate are
  all managed from the page — no shelling into the relay host.
- Credentials issued in the panel take effect immediately; revoking a machine
  drops its tunnel without a restart.
- Browser tokens can be minted with a lifetime (1 hour to 30 days, or never) and
  rotated from the page.

**Daemon (`tfd`)**

- Owns real PTY sessions on the machine it runs on, and dials out to a relay so
  the machine needs no inbound port.
- Runs PowerShell by default on Windows when a modern one is installed, falling
  back to Windows PowerShell and only then `cmd.exe`.
- `-relay-ca` pins a private CA, which is what a relay behind a self-signed
  certificate requires.

**Desktop (`tfdesktop`)**

- Native window (mygo UI) with a terminal per session, plus a settings window.
- Terminal font family and size, applied to already-open tabs.
- Relay address, device id and secret, CA certificate (`Fetch from server` or a
  file picker) with a live connection status line.
- Local loopback service toggle, so a browser on the same machine can reach the
  same sessions.

**Project**

- `resources/icon.png` is now a real application icon, so Linux desktop entries,
  macOS bundles and Windows executables are branded.
- Release workflow builds desktop installers and command-line tools for macOS,
  Linux and Windows.
