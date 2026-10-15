# Changelog

Notable changes per release. The release workflow lifts the section matching the
pushed tag straight into the GitHub release notes, so keep the headings exactly
`## <version>`.

## Unreleased

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
