<p align="center">
  <img alt="Beacon" src=".github/assets/logo.png" width="140" height="140">
</p>

<h1 align="center">Beacon</h1>

<p align="center">
  <strong>One terminal hub, controlled from anywhere. Signaled when it needs you.</strong>
</p>

<p align="center">
  <a href="https://github.com/CarlOpenLab/beacon/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/CarlOpenLab/beacon/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/CarlOpenLab/beacon/releases/latest"><img alt="Latest Release" src="https://img.shields.io/github/v/release/CarlOpenLab/beacon?color=6366f1"></a>
  <img alt="Platforms" src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-blue">
  <img alt="Go Version" src="https://img.shields.io/badge/go-%3E%3D1.24-00ADD8">
</p>

<p align="center">
  <a href="README.md">English</a> • <a href="README.zh_CN.md">简体中文</a>
</p>

---

**Beacon** turns the real PTY sessions across your physical and virtual machines into an ambient, unified terminal hub.

> This project used to be `term-fleet`. The repository and the Go module were renamed to `beacon`; GitHub redirects the old links, and an existing settings directory under `term-fleet` is still read (`TERM_FLEET_CONFIG_HOME` keeps working).

A lightweight daemon (`tfd`) manages PTY sessions over a single multiplexed WebSocket. A native desktop app built with Mygo (featuring floating cards and soft color washes, rendered natively without WebViews), a browser client, and a mobile client all attach to the exact same connection. Every shell tab on your phone is the identical shell running on your desktop.

Machines dial **out** to a public relay (`tfrelay`), making all your sessions reachable from outside your local network without port forwarding, dynamic DNS, or a VPN.

```
       Desktop App                     Phone / Browser
  (Native Mygo Window)              (Responsive Web Client)
           │                                   │
           │  wss://                           │  wss:// (first-frame token)
           ▼                                   ▼
   ┌────────────────────────────────────────────────────────┐
   │                 Public Relay (tfrelay)                 │
   │           Routes bytes · NAT/CGNAT Traversal           │
   └────────────────────────────────────────────────────────┘
                               ▲
                               │  wss:// (members dial OUT)
            ┌──────────────────┴──────────────────┐
            │                                     │
     Local Machine 1                       Remote Server 2
┌───────────────────────┐             ┌───────────────────────┐
│       Agent (tfd)     │             │       Agent (tfd)     │
│   PTY / ConPTY Hub    │             │   PTY / ConPTY Hub    │
│  Ring Buffer History  │             │  Ring Buffer History  │
└───────────────────────┘             └───────────────────────┘
```

---

## Key Highlights

- **Seamless Multi-Client Attachment**  
  Not another VNC or remote desktop. Beacon shares real PTY sessions with a single-controller, multi-viewer lease model. A phone in your pocket can mirror what the desktop is doing without accidental input thrashing, and take over the keyboard with a single tap.
- **Ambient State & Attention Awareness**  
  Beacon parses terminal titles, bells, spinners, and structured program reports (`OSC 7501`) into live status glyphs: `working`, `waiting`, `idle`, `error`, and `exited`. When a build fails, a test completes, or an AI coding agent stops to ask for your permission, Beacon alerts you via Dock badges and notifications.
- **Detached, Never Destroyed**  
  Closing a laptop lid or losing Wi-Fi is just a disconnect, never a killed process. Sessions and their ring-buffered output history live inside the host daemon. When you reconnect, you pick up right from your previous byte offset — no replay storms, no lost output.
- **Zero-Inbound NAT Traversal**  
  Nodes establish outbound WebSocket tunnels to your relay. Your machines stay safely behind NAT, firewalls, and CGNAT without opening a single incoming port.
- **Native Desktop Craftsmanship**  
  The desktop client is a native Go app with floating window cards, translucent color washes, fluid split-panes, directory-inheriting tabs, a fuzzy command palette (`⌘P`), and quick navigation to waiting tasks (`⌘J`).

---

## Comparison

| Capability | **Beacon** | SSH | tmux | tmate |
| :--- | :---: | :---: | :---: | :---: |
| **No Inbound Port Required** | **Yes** (Dial-out relay) | No (Needs public IP/NAT) | N/A (Local) | Yes (Third-party server) |
| **Simultaneous Desktop & Mobile** | **Yes** (Native + Mobile web) | Extra setup (Mosh/Termius) | Terminal only | Web view only |
| **Disconnect Resilience** | **Yes** (Byte-offset resume) | Drops with connection | Preserved in session | Preserved on host |
| **Program State Sensing** | **Yes** (OSC 7501 / Bell / Wait) | None | None | None |
| **Smart Split (Inherits `cwd`)** | **Yes** (Reads `/proc` & `lsof`) | Manual | Manual configuration | Manual |
| **Native GUI Experience** | **Yes** (Floating cards, wash themes) | Terminal only | Terminal only | Terminal only |

---

## Download & Installation

Pre-built binaries and installers for every release are available on the **[Releases Page](https://github.com/CarlOpenLab/beacon/releases)**:

| Platform | Package Type | Details |
| :--- | :--- | :--- |
| **macOS** | `.dmg` | Universal binary (Apple Silicon & Intel) |
| **Windows** | `Setup.exe` / `.zip` | x64 installer & portable archive (ConPTY backed) |
| **Linux** | `.deb` / `.tar.gz` | Debian, Ubuntu, and generic archive (x86_64, arm64) |
| **CLI Tools** | `.tar.gz` / `.zip` | Standalone `tfd` (agent) and `tfrelay` (relay) |

> [!NOTE]
> Release assets are currently self-signed. On macOS, right-click the app and choose **Open**. On Windows, click **More info → Run anyway** if SmartScreen appears.

---

## Quick Start

### 1. Run Locally (LAN / Same Machine)

If you only want to access your terminal from your local browser or phone on the same Wi-Fi:

```sh
# Clone and build
git clone https://github.com/CarlOpenLab/beacon.git
cd beacon
pnpm install
pnpm run build

# Start the daemon (serves browser UI on 127.0.0.1:7678)
./bin/tfd

# Launch the native desktop window (uses the same daemon)
./bin/tfdesktop
```

Open `http://localhost:7678` in any browser or visit from your phone (using your LAN IP with `-addr 0.0.0.0:7678 -token <secret>`).

---

### 2. Connect from Anywhere via Relay

To access your machines remotely over the internet:

#### Step A: Run the Relay (Public Server)
```sh
./bin/tfrelay -addr 0.0.0.0:7698 \
              -client-token "YOUR_CLIENT_TOKEN" \
              -public-url "wss://relay.example.com/relay/agent" \
              -devices devices.json
```
*(The relay includes a built-in admin dashboard at `https://relay.example.com/admin` to enroll machines, issue tokens, and inspect remote logs).*

#### Step B: Join a Machine to the Relay
```sh
./bin/tfd -relay "wss://relay.example.com/relay/agent" \
          -device-id "laptop-carl" \
          -device-secret "DEVICE_SECRET_FROM_ADMIN"
```

Once joined, opening `https://relay.example.com/` in any browser or phone connects directly to all your enrolled machines.

---

## Desktop Keybindings

The desktop client is optimized for seamless keyboard navigation:

| Shortcut | Action |
| :--- | :--- |
| `⌘P` / `Ctrl+P` | Open Command Palette (search sessions, machines, and actions) |
| `⌘J` / `Ctrl+J` | Jump directly to the next session that **needs attention** |
| `⌘T` / `Ctrl+T` | New terminal session (inherits the current tab's active directory) |
| `⌘D` / `Ctrl+D` | Split active pane vertically (side-by-side) |
| `⌘⇧D` / `Ctrl+Shift+D`| Split active pane horizontally (top / bottom) |
| `⌘[` / `⌘]` | Navigate between split panes |
| `⌘⇧[` / `⌘⇧]` | Switch between session groups |
| `⌘1` … `⌘9` | Switch directly to the N-th session |
| `⌘⇧↩` / `Ctrl+Shift+Enter` | Zoom / toggle full-frame on the focused pane |
| `⌘R` / `Ctrl+R` | Rename active session title |
| `⌘W` / `Ctrl+W` | Close active pane (terminates process if last view) |

*All shortcuts can be rebound in **Settings → Keyboard**.*

---

## Program-Driven State (`OSC 7501`)

Beacon allows CLI utilities, background tasks, and AI coding agents to communicate their state back to the UI. A script can emit an `OSC 7501` escape sequence to update its card badge and sidebar glyph:

```sh
# Report permission prompt waiting for user response:
printf '\033]7501;state=waiting:kind=permission:msg=%s\033\\' "$(printf 'Approve execution?' | base64)"

# Report progress:
printf '\033]7501;state=working:progress=45:msg=%s\033\\' "$(printf 'Running migrations...' | base64)"

# Clear back to idle:
printf '\033]7501;state=idle\033\\'
```

Beacon also monitors standard bell signals (`\a`), OSC 9 notifications, terminal title changes, and active stdout stream status to infer state automatically.

---

## Core Architectural Principles

1. **Attaching is not Creating**  
   Disconnecting (laptop lid closed, Wi-Fi dropped, browser closed) merely detaches the viewer. The process continues running safely on the host. Reattaching replays only the delta bytes missed.
2. **Single Controller, Multiple Observers**  
   A terminal PTY has exactly one window size and one stdin stream. To prevent input races and resize thrashing between devices, only one client holds write/resize control at a time. Other clients act as synchronized read-only mirrors until granted or taking control.
3. **Closing a Tab Ends its Process**  
   Clicking `×` or pressing `⌘W` on the desktop terminates the shell process on the host machine. The browser client distinguishes between *Close View* (detaching) and *End Process* (killing).

---

## Configuration & Environment Variables

The desktop client persists configuration to `settings.json` located in your OS configuration directory:

- **macOS**: `~/Library/Application Support/beacon/` (falls back to `term-fleet`)
- **Windows**: `%AppData%\beacon\`
- **Linux**: `~/.config/beacon/`

### Environment Variables
- `BEACON_CONFIG_HOME`: Custom path to the settings and logs directory (takes precedence).
- `TERM_FLEET_CONFIG_HOME`: Legacy config directory override for backwards compatibility.

---

## Development & Testing

```sh
# 1. Install frontend dependencies and build assets
pnpm --dir web install
pnpm run build:web

# 2. Run Go tests across all packages
pnpm run test        # or: go test ./...

# 3. Static checks and TypeScript verification
pnpm run check       # go vet ./... && tsc --noEmit

# 4. End-to-end browser & relay acceptance tests
pnpm run acceptance:all
```

### Project Layout
```
cmd/tfd/               Headless host agent (owns PTYs, serves WebSocket)
cmd/tfdesktop/         Native desktop client (Mygo GUI toolkit)
cmd/tfrelay/           Public relay server (router, auth, presence)
internal/session/      PTY abstraction (Unix PTY + Windows ConPTY) & ring buffer
internal/hub/          WebSocket fan-out & lease arbitrator
internal/agent/        Dial-out client connecting host to relay
internal/relay/        Public router & member registry
internal/uitokens/     Design tokens (colors, soft wash backgrounds)
web/                   Web & mobile client (TypeScript + xterm.js)
deploy/                Systemd units, Caddy reverse-proxy config & security runbook
```

---

## License

This project is licensed under the [MIT License](LICENSE).
