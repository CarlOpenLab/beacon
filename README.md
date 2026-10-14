# term-fleet

One terminal fleet, controlled from anywhere on your own network.

`tfd` owns the PTY sessions on a machine and serves them over a single
WebSocket. A desktop client built with Mygo (Go + a system WebView) and a
mobile client both attach to that same connection, so every terminal tab on
the phone is the same shell as the tab on the desktop.

Design notes and the protocol draft live in [PLAN.md](PLAN.md).

## Status

M0 (the walking skeleton) is done: PTY sessions, byte-offset output history,
one multiplexed WebSocket, and a single-controller model with viewers.

Not built yet: the desktop client, the mobile client, Windows hosts, and
transport hardening.

## Layout

```
cmd/tfd/               the daemon
internal/protocol/     the wire format, shared with clients
internal/session/      PTY, output ring buffer, session manager
internal/hub/          WebSocket fan-out and who-may-type policy
```

## Run

```sh
go test ./...
go build -o bin/tfd ./cmd/tfd
./bin/tfd                      # listens on 127.0.0.1:7678
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

Endpoints: `GET /healthz`, `GET /ws` (WebSocket).

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
