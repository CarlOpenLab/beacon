# Changelog

Notable changes per release. The release workflow lifts the section matching the
pushed tag straight into the GitHub release notes, so keep the headings exactly
`## <version>`.

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
