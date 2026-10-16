# Changelog

Notable changes per release. The release workflow lifts the section matching the
pushed tag straight into the GitHub release notes, so keep the headings exactly
`## Unreleased

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
- The window is now a 56px strip (the mark, `切换到白天/暗黑`, `设置`, and the
  service's state at the bottom), a header naming the shell on screen with
  `新建标签页` / `关闭标签页`, and a bottom line naming the service and the build.
  The column is left to the shells alone.
- `docs/takeover.md`: taking over a shell on another machine — one writer with an
  epoch'd lease, the glow and 拿回控制权 popover, and input under latency.

## <version>`.

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
