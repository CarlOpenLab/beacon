# Changelog

Notable changes per release. The release workflow lifts the section matching the
pushed tag straight into the GitHub release notes, so keep the headings exactly
## Unreleased

- The shell window has a left rail: the open shells, `切换到白天/暗黑`, and the
  settings window. The appearance is saved (`theme`: `light`, `dark`, or unset
  for the system) and the terminal follows it.
- The settings window has an 外观 pane with all three modes.
- New mark, taken from the logo sheet's third row, second column: the app icon
  keeps the sheet's dark tile, the README logo and the web favicon keep the white
  tile with dark ink. `resources/logo.svg` and `web/public/favicon.svg` are gone
  in favour of `favicon.png`.
- The desktop window has a palette of its own — the navy ground, the violet for
  what is chosen and the green for what is live — instead of the ui's default
  greys, and the terminal is themed in the same palette rather than its own.
- The window is now a 56px strip (the mark at the top, `切换到白天/暗黑` and
  `设置` at the bottom over the service's state), a column beside it holding the
  会话 and 机器 panels with a draggable line between them, a header naming the
  shell on screen, and a bottom line naming the service and the build. The
  window's body is left to the shell alone.
- `docs/takeover.md`: taking over a shell on another machine — one writer with an
  epoch'd lease, the glow and 拿回控制权 popover, and input under latency.

**UI**

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
  service's state, and the relay it is enrolled with — its address, this
  machine's device id, and the link's state. Other machines join the list the
  day the window can drive their shells; the panel says so until then, rather
  than pretending the list is complete.
- The settings window is a form: every pane's labels sit in a column beside
  their controls, right-aligned as macOS forms do, instead of stacked full-width
  above them. 应用 is offered only when there is something to apply, the panes
  scroll instead of clipping in a short window, and the 更新 pane hides the
  check button on a build that cannot update itself rather than offering a
  control that does nothing.
- A window flashes the colour of the appearance it is about to paint, so a light
  window no longer opens with a dark frame around it.

**Browser client**

- Selecting text in a terminal now has the affordance a text surface has: drag a
  range and a 复制 button appears at it — the copy is a choice, not something
  that happens because the pointer was released. Right-click copies the
  selection too, since xterm paints selection in its own layer and the
  browser's own Copy has nothing to act on. `⌘C` / `Ctrl+Shift+C` work as
  before.

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
