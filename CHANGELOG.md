# Changelog

Notable changes per release. The release workflow lifts the section matching the
pushed tag straight into the GitHub release notes, so keep the headings exactly
`## <version>`.

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
