# term-fleet

One terminal fleet, controlled from anywhere.

`tfd` owns the PTY sessions on a machine and serves them over a single
WebSocket. A desktop client built with Mygo (Go + a system WebView) and a
mobile client both attach to that same connection, so every terminal tab on
the phone is the same shell as the tab on the desktop.

Machines can also dial *out* to a relay (`tfrelay`) on a public server, so the
fleet is reachable from outside the LAN without a port forward, a public
address, or a VPN. The relay routes bytes and nothing else — see
[PLAN.md](PLAN.md) §3.

Design notes and the protocol draft live in [PLAN.md](PLAN.md).

## Status

M0 through M5 are done: the walking skeleton, the PC client, the relay, the
client's tunnel mode, Windows hosts, and the deployment pieces.

- PTY sessions, byte-offset output history, one multiplexed WebSocket, and a
  single-controller model with viewers.
- The browser client: tabs, session inventory, keyboard-driven new/close,
  copy, reopen-from-cursor after a dropped link, and a machine-grouped sidebar
  when more than one machine is reachable.
- It is served by the daemon *and* by the relay, from one build: the server
  reports which transport it speaks (`/tf/config`), so the desktop shell, a
  browser tab and a phone all load the same code.
- `tfdesktop`: the same daemon behind a Mygo window.
- `tfrelay` + `agent`: machines dial out to a public relay and are controllable
  from anywhere; a dropped link is a routing outage, not data loss — sessions
  and their output history live in the member and survive it.
- Windows hosts, through ConPTY: `internal/session` abstracts a pty behind one
  interface, because a Unix pty is a file descriptor and a ConPTY is a console
  object that owns the process it launched.
- A phone-shaped client: a drawer sidebar, and an accessory key bar for the
  keys a soft keyboard does not have.

`deploy/` holds a systemd unit for the relay, a Caddyfile that terminates TLS
in front of it, a user unit for members, and a runbook covering the security
model. The relay rate-limits failed credentials per address and takes the
client token in the first frame rather than in the URL.

## Layout

```
cmd/tfd/               the daemon
cmd/tfdesktop/         the desktop shell: same daemon, plus a Mygo window
cmd/tfrelay/           the public-side router: enroll members, route streams
internal/protocol/     the wire format, shared with clients
internal/tunnel/       the relay envelope; the session protocol rides inside it
internal/session/      PTY, output ring buffer, session manager
internal/hub/          WebSocket fan-out and who-may-type policy
internal/relay/        member registry, stream routing, presence
internal/agent/        dials out to a relay, projects streams onto the hub
internal/daemon/       hub + client wiring shared by both entry points
internal/web/          go:embed of the built client
web/                   the client itself (Vite + TypeScript + xterm)
deploy/                systemd units, Caddyfile, and the deployment runbook
scripts/acceptance.mjs headless end-to-end run against a real browser
scripts/relay-acceptance.mjs  the same, across a relay, against the real binaries
scripts/relay-browser.mjs     a real browser through a relay, as desktop and as phone
```

## Run

The client is embedded with `go:embed`, so build the web step first. A bare
`go build` still succeeds and serves a page telling you to do this.

```sh
pnpm --dir web install
pnpm run build            # web + tfd + tfdesktop into bin/
./bin/tfd                 # listens on 127.0.0.1:7678, serves the client at /
./bin/tfdesktop           # same thing in a window, on a free loopback port
```

For UI work, skip the recompile: build the assets and point the daemon at them.

```sh
pnpm run build:web
./bin/tfd -addr 127.0.0.1:7678 -web-dir internal/web/dist
```

`tfd` refuses to listen on a non-loopback address unless you pass `-token`.
It runs arbitrary commands and serves a shell; that is not a service to expose
by accident.

| flag | meaning |
| --- | --- |
| `-addr` | listen address, loopback by default |
| `-token` | shared secret every client must present |
| `-allow-origin` | extra WebSocket Origin patterns, for a dev server on another port |
| `-idle-controller` | how long a silent controller keeps control (default 30m) |
| `-web-dir` | serve the client from disk instead of the embedded copy |
| `-relay` | dial this relay member endpoint; empty keeps the machine LAN-only |
| `-device-id` / `-device-secret` | this machine's relay credential |
| `-machine-name` | the name shown in the relay's machine list |

Endpoints: `GET /` (client), `GET /healthz`, `GET /ws` (WebSocket).

## Reach it from anywhere

A relay is a router, not a hub. It never parses a session frame, never holds a
PTY, and never decides who controls a session; it pairs a client stream with a
member stream and forwards bytes. Members dial out, so nothing listens on the
machine itself.

```sh
./bin/tfrelay enroll -name "mac mini"     # prints a credential, writes devices.json (0600)
./bin/tfrelay list                        # who is enrolled

./bin/tfrelay -addr 127.0.0.1:7698 -devices devices.json \
              -client-token "$TOKEN" -tls-cert cert.pem -tls-key key.pem

./bin/tfd -addr 127.0.0.1:7678 \
          -relay wss://relay.example/relay/agent \
          -device-id mac-mini-1a2b3c -device-secret "$SECRET"
```

Each machine gets its own credential rather than one shared fleet secret, so a
laptop can be revoked without re-enrolling everything. The relay refuses to
start with no devices or no client token, and refuses to listen off loopback
without TLS unless you say `-insecure` — the same stance `tfd` takes, for the
same reason: it fronts arbitrary command execution.

The relay also serves the client bundle, so one URL is the whole thing: open
`https://relay.example/` and it asks for the token, then lists every enrolled
machine with its sessions. The token is sent in the first frame, never as a
query parameter — a URL ends up in access logs, in intermediate proxies and in
browser history, none of which should hold the key to every machine. Failed
attempts are rate-limited per address, and the address is taken from the last
`X-Forwarded-For` hop (the one a proxy appends) so a forged header cannot
invent a fresh identity per try.

`deploy/README.md` is the full runbook: build, token, systemd, TLS, enrolling
machines, verification, upgrades, revocation, and an honest account of what the
relay can see (everything — it terminates TLS and carries your keystrokes).

## Tests

```sh
pnpm run test          # go test ./...
pnpm run check         # go vet + tsc --noEmit
pnpm run acceptance    # builds, then drives real Chrome against a real daemon
pnpm run acceptance:relay  # real tfrelay + tfd binaries, a session across the tunnel
pnpm run acceptance:relay-browser  # a real browser through the relay, desktop and phone
pnpm run acceptance:all            # all three
```

`scripts/acceptance.mjs` starts `bin/tfd`, opens two Chrome pages over CDP, and
asserts the things that only break in a browser: the prompt renders, typing
reaches the shell, `⌘T` opens a second tab, closing a tab detaches without
killing the session, a dropped socket reconnects without replaying the screen,
and a second client attaches read-only, cannot type, and can take control.

`scripts/relay-acceptance.mjs` runs the real `tfrelay` and `tfd` binaries,
enrolls a machine, and drives a shell through the tunnel. It exists for the
failures the in-process Go tests cannot see: flag wiring, the enroll →
configure handshake, and the base64 layering a client has to get right (a
tunnel payload is bytes, and so is a session frame's data).

`scripts/relay-browser.mjs` is the one that checks what the phone actually
runs: Chrome, through a real relay, with a real member behind it. It connects
with a token, creates a session, types into it, then opens a second page with
phone metrics and a `pointer: coarse` media override. That second page has to
see the *same session id*, attach read-only, refuse a touch key while it is a
viewer, take over, and then drive the host. A scripted WebSocket client cannot
prove any of that — only the real client can.

On Windows, `go test ./internal/session` is worth running natively: it starts a
ConPTY, runs a command through it, resizes it, and checks the exit code. None of
that can be verified by cross-compiling.

The Go side covers the routing itself, including the case that matters most:
`TestAgentRedialsAfterLinkLoss` drops every link the relay holds, waits for the
member to redial on its own, and asserts a session created before the outage is
still alive and attachable afterwards.

## Two rules that shape everything

**Attaching is not creating.** A client that disconnects — phone slept, Wi-Fi
dropped, laptop lid closed — is detached, never closed. The shell keeps
running and the client reattaches by cursor, receiving only the bytes it
missed.

**One controller per session.** A PTY has one size and one stdin. If every
client could resize, the screen would thrash as the desktop and the phone
fought over cols/rows; if every client could type, a phone in a pocket would
interfere with real work. So a session has at most one controller that may
type, resize, and close, and any number of viewers that get a read-only
mirror. Input from a viewer is refused with `not_controller` rather than
dropped, so a client can offer to take control. Control is taken with `claim`
and given back with `release`, and a controller that sits silent for
`-idle-controller` loses it automatically.

**Closing a tab is not killing a session.** The tab's `×` detaches — the shell
keeps running and stays in the session list, which is what you want when you
were only peeking. Ending the process is a separate button, because that one is
irreversible for every other client looking at it.
