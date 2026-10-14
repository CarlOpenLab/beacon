# term-fleet

One terminal fleet, controlled from anywhere on your own network.

`tfd` owns the PTY sessions on a machine and serves them over a single
WebSocket. A desktop client built with Mygo (Go + a system WebView) and a
mobile client both attach to that same connection, so every terminal tab on
the phone is the same shell as the tab on the desktop.

Design notes and the protocol draft live in [PLAN.md](PLAN.md).

## Status

M0 (walking skeleton) and M1 (PC client) are done.

- PTY sessions, byte-offset output history, one multiplexed WebSocket, and a
  single-controller model with viewers.
- The browser client: tabs, session inventory, keyboard-driven new/close,
  copy, reopen-from-cursor after a dropped link.
- It is served by the daemon itself, so the desktop shell, a browser tab, and
  (in M2) a phone all load the same code from one origin.
- `tfdesktop`: the same daemon behind a Mygo window.

Not built yet: the mobile client, Windows hosts (ConPTY), and transport
hardening (token auth over TLS for real LAN use).

## Layout

```
cmd/tfd/               the daemon
cmd/tfdesktop/         the desktop shell: same daemon, plus a Mygo window
internal/protocol/     the wire format, shared with clients
internal/session/      PTY, output ring buffer, session manager
internal/hub/          WebSocket fan-out and who-may-type policy
internal/daemon/       hub + client wiring shared by both entry points
internal/web/          go:embed of the built client
web/                   the client itself (Vite + TypeScript + xterm)
scripts/acceptance.mjs headless end-to-end run against a real browser
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

Endpoints: `GET /` (client), `GET /healthz`, `GET /ws` (WebSocket).

## Tests

```sh
pnpm run test          # go test ./...
pnpm run check         # go vet + tsc --noEmit
pnpm run acceptance    # builds, then drives real Chrome against a real daemon
```

`scripts/acceptance.mjs` starts `bin/tfd`, opens two Chrome pages over CDP, and
asserts the things that only break in a browser: the prompt renders, typing
reaches the shell, `⌘T` opens a second tab, closing a tab detaches without
killing the session, a dropped socket reconnects without replaying the screen,
and a second client attaches read-only, cannot type, and can take control.

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
