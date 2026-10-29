# Deploying Beacon

Three roles, one binary each:

| Role | Binary | Where it runs | Reaches |
|---|---|---|---|
| **relay** | `tfrelay` | a public server | the internet |
| **member** | `tfd` | each machine you want to reach | outbound only to the relay |
| **client** | the web bundle | served by the relay | the relay |

Members dial **out**. That is the point of the design: no public address, no port
forwarding, and it works behind NAT and CGNAT.

```
        phone / laptop                 relay (public)                your machines
  ┌──────────────────────┐        ┌─────────────────────┐       ┌──────────────────┐
  │  browser → wss://    │───────▶│   /relay/client     │       │  tfd ← ConPTY    │
  │  relay, first-frame  │        │        │            │◀──────│  tfd ← PTY       │
  │  token               │◀───────│   routing table     │ out   │  (dial out)      │
  └──────────────────────┘        │        │            │       └──────────────────┘
                                  │   /relay/agent ◀────┼── wss (member dials out)
                                  └─────────────────────┘
```

The relay routes bytes and nothing else. It never parses a session frame, never
holds a PTY, and never decides who controls a session. Everything the session
protocol means stays inside the machine's own daemon — which is why a session
keeps running when the relay restarts.

---

## 1. Build

From a checkout, on any machine with Go 1.24+ and Node 20+:

```sh
pnpm install
pnpm run build:web          # the client bundle, embedded into the binaries
pnpm run build:relay        # → bin/tfrelay
go build -o bin/tfd ./cmd/tfd
```

`build:web` must run before the Go builds: the client is baked in with
`go:embed`, so building the binary first gives you a binary with the previous
client inside it.

## 2. Create the client token

One token, shared by every client you will connect with:

```sh
head -c 32 /dev/urandom | base64 | tr -d '=' | tr '/+' '_-'
```

Use a real random value. This token is the only thing between the open internet
and a shell on every enrolled machine.

## 3. Install the relay

On the server (Debian/Ubuntu shown; adapt for your distro):

```sh
sudo useradd --system --home /var/lib/tfrelay --shell /usr/sbin/nologin tfrelay

sudo install -m 0755 bin/tfrelay /usr/local/bin/tfrelay
sudo install -d -m 0700 -o tfrelay -g tfrelay /var/lib/tfrelay /etc/tfrelay

sudo install -m 0600 -o tfrelay -g tfrelay /dev/stdin /etc/tfrelay/env <<'EOF'
TFRELAY_CLIENT_TOKEN=REPLACE_WITH_THE_TOKEN_FROM_STEP_2
# Optional: bootstrap the super admin at startup (only when no account exists).
#TFRELAY_ADMIN_USER=carl
#TFRELAY_ADMIN_PASSWORD=REPLACE_WITH_A_LONG_PASSWORD
EOF

sudo install -m 0644 deploy/tfrelay.service /etc/systemd/system/tfrelay.service
sudo systemctl daemon-reload
sudo systemctl enable --now tfrelay
```

The registry file is created on first use; keep it at mode 0600 — the member
secrets in it are as sensitive as the machines they unlock.

The relay also writes `tfrelay.log` beside the registry by default, and keeps
one `tfrelay.log.1` backup after the active file reaches 8 MiB. Machine log
reports are stored under `logs/machines/` as one bounded JSON report per device
so the admin Logs page keeps its history across a relay restart. Override both
locations with `-log-dir`; the service unit uses `/var/lib/tfrelay/logs`.
The log directory and files are private to the relay account (0700/0600).

# 3b. Set up the admin panel

Open the relay's management page once and create the admin account:

```
https://relay.example.com/admin        # or https://host/termfleet/admin/ behind a prefix
```

It is where machines are enrolled, the client token is minted and read, and the
CA certificate is downloaded. The account is stored hashed at
`/var/lib/tfrelay/admin.json` (0600, override with `-admin-store`); losing the
password means deleting that file and re-running setup. Turn the page off
entirely with `-admin=false` if you would rather keep setup in the shell.

The client token is a single shared secret, so it is also the one credential
worth giving an expiry: pick a lifetime on the page — 1 hour to 30 days, or
"never" — and the default is 24 hours. When it lapses, viewers are refused until
someone pastes a fresh token; the page keeps showing how long the current one
has left. Set the default with `-client-token-ttl` (`0` means tokens never
expire), and note that a token passed with `-client-token` never expires at all,
since it was configured by hand.

Give it a non-default path with `-admin-path`, and a real relay URL to hand out
with `-public-url wss://relay.example.com/relay/agent` — the relay cannot guess a
URL that a proxy in front is rewriting.

# 3c. Multi-tenant: signups, approval, tenant isolation

By default the client token makes the relay one fleet. With the user store on —
the default — the relay also serves an open account API, and machines are
enrolled into tenants:

- **Signup and login** live at `/api/register` and `/api/login`. A signup lands
  in the admin panel's **Users** page as pending, and cannot log in until you
  approve it there.
- **An approved user is one tenant.** What they enroll is theirs alone: their
  browser console shows only their machines, their desktop client reaches only
  their machines, and a machine's own short-lived credential stays inside the
  tenant too. The shared token, this panel, and named tokens minted here keep
  their operator-wide reach.
- **Self-serve enrollment.** An approved user clicks 添加机器 in the web client
  and gets a device id/secret to paste into that machine's desktop app, without
  the admin panel involved.

Bootstrap the super admin at deploy time so approval is possible before anyone
opens the panel — put `TFRELAY_ADMIN_USER` and `TFRELAY_ADMIN_PASSWORD` in
`/etc/tfrelay/env` (or pass `-admin-user`/`-admin-password`). They are read once
at startup and only ever create the account when none exists; without them the
setup wizard makes the first account instead, exactly as before.

`-user-token-ttl` (default 7d) bounds a login token's life. Rejecting or
deleting a user in the panel ends their tokens at once. Pass `-users off` to
turn the whole account surface back off on a single-operator relay.

### TLS

**With a proxy (recommended).** Install Caddy, copy `deploy/Caddyfile`, replace
the hostname, and reload. Caddy obtains and renews the certificate. The relay
stays on loopback with `-trust-proxy`.

```sh
sudo cp deploy/Caddyfile /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

**Without a proxy.** Point the relay at a certificate directly and drop
`-trust-proxy`:

```sh
tfrelay -addr :7699 -devices /var/lib/tfrelay/devices.json \
        -client-token "$TOK" \
        -tls-cert /etc/letsencrypt/live/relay.example.com/fullchain.pem \
        -tls-key  /etc/letsencrypt/live/relay.example.com/privkey.pem
```

Either way, open only 443 (and 80, for the ACME challenge). The relay refuses to
start on a non-loopback address without TLS and without `-insecure`.

**No domain: share 80/443 with an existing site.** If the box already serves
something else on 80/443 (Caddy, nginx) and you have no hostname to point at
the relay, give the relay its own prefixes inside that existing vhost instead
of a new port. The client bundle, its runtime config and the WebSocket tunnels
each need a prefix, because they assume they live at the root of a host:

```nginx
# /termfleet/  -> the bundled client      (prefix stripped)
location ^~ /termfleet/ { proxy_pass http://127.0.0.1:7699/; }

# /tf/ and /relay/ are called as absolute paths by the client, so they must
# reach the relay untouched.
location ^~ /tf/    { proxy_pass http://127.0.0.1:7699; }
location ^~ /relay/ {
    proxy_pass http://127.0.0.1:7699;
    proxy_http_version 1.1;
    proxy_set_header Upgrade    $http_upgrade;
    proxy_set_header Connection "upgrade";
}
```

People then open `https://host/termfleet/`, while members still dial
`wss://host/relay/agent`. Keep `-trust-proxy` on the relay, and forward
`X-Forwarded-For`. Only the three prefixes above are touched; the existing site
keeps "/".

The admin panel sits under the same prefix (`https://host/termfleet/admin/`)
because its pages and API calls are relative to where the page is loaded. That is
also why the relay answers `…/admin` and `…/admin/` rather than redirecting: a
redirect to an absolute `/admin/` would escape the prefix and 404 against the
other site.

One thing a prefixing proxy must tell the relay is *what* it stripped, because
the panel's session cookie is scoped to a path the browser uses, not the one the
relay sees:

```nginx
location ^~ /termfleet/ {
    proxy_pass http://127.0.0.1:7699/;
    proxy_http_version 1.1;
    proxy_set_header X-Forwarded-Prefix /termfleet;   # the browser's prefix
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    ...
}
```

Without it, signing in appears to succeed and then every following request is
anonymous: the cookie was scoped to `/admin`, which the browser never visits.

**Private CA or self-signed certificate.** A member verifies the relay's TLS
certificate against the system roots, so a self-signed certificate (or one from
a private CA) fails with `x509: certificate signed by unknown authority`. Point
the member at the certificate instead of installing it system-wide:

```sh
./bin/tfd -addr 127.0.0.1:7678 \
          -relay wss://relay.example/relay/agent \
          -relay-ca /etc/ssl/relay.crt \
          -device-id mac-mini-1a2b3c -device-secret "$SECRET"
```

`-relay-ca` takes a PEM file (the CA certificate, or the self-signed leaf) and
makes it the only root the dial accepts, so a wrong certificate fails closed
rather than falling back to the system store. The desktop shell has the same
field as "CA 证书" on its 远程 settings tab. Browsers do not need it — they
show a one-time warning for a self-signed certificate.

## 4. Enroll each machine

In the admin panel, add the machine. It shows a device id and a secret **once**;
run the member with them:

```sh
tfd -addr 127.0.0.1:7677 \
    -relay wss://relay.example.com/relay/agent \
    -device-id  <id> \
    -device-secret <secret>
```

Give each machine its own credential. That is what makes revocation mean
something: a machine you sell, wipe or lose can be crossed out without touching
the others. Revoke from the panel — it takes effect on the next dial, and needs
no restart. The registry is read live rather than snapshotted at startup, which
is also what lets a machine enrolled after the relay started connect right away.

For scripted setup the CLI still works, and writes the same file:

```sh
sudo -u tfrelay tfrelay enroll -devices /var/lib/tfrelay/devices.json -name "mac-mini"
```

A relay with no machines at all now starts, so that this step can come after
installation: an empty registry is inert, because nothing can dial in without a
credential.

### Member as a service

A member that is not already running when the phone reaches for it is a machine
that is offline. Install it as the machine's own service, with the same
enrollment flags you would have typed:

```sh
tfd service install \
  -relay wss://relay.example.com/relay/agent \
  -device-id <id> -device-secret <secret>
tfd service status
```

That writes a systemd user unit on Linux (and enables linger, so it survives
logout), a launchd agent on macOS, or a logon task on Windows — whichever this
machine has — and starts it. `tfd service restart` restarts it (its sessions end
with it), and `tfd service uninstall` removes it. The credential lives in that
service file at mode 0600; keep it as private as the `tfd` invocation itself.

Re-running `install` with a newer binary replaces it and restarts the service.

### Install without a checkout

```sh
curl -fsSL https://raw.githubusercontent.com/CarlOpenLab/beacon/main/scripts/install-member.sh \
  | sh -s -- -relay wss://relay.example.com/relay/agent \
             -device-id <id> -device-secret <secret>
```

It downloads the latest `beacon-cli` release for this OS and architecture into
`~/.local/bin`, then runs `tfd service install` with the flags after `--`. Run
the same line again to update; the enrollment is unchanged. On Windows use
`scripts/install-member.ps1` (see its header for the parameters).

By default the member also binds its loopback port, so a browser on the same LAN
can skip the relay entirely; the relay path is what makes it reachable from
outside.

### Checking which build a machine runs

`tfd version` prints the build. The desktop window names it in the machine
panel's tooltip, and the relay's roster carries it, so a machine that is behind
is visible before it is a problem. Updating is the same install line as above:
the relay and its members do not have to be upgraded together, because features
are negotiated rather than versioned into the handshake.

## 5. Verify

```sh
curl -fsS https://relay.example.com/healthz
# {"ok":true,"online":2}
```

Then open `https://relay.example.com/` in a browser. It will ask for the client
token, and remember it on that device. Nothing is sent until you press 连接.

`online` counts connected members. Zero means the relays are up and no machine
is dialling in — check the member logs before the relay's.

## 6. Upgrade

The relay and its members do not have to be upgraded together: the envelope is
versioned by frame type, and the session protocol inside a stream is negotiated
between the client and the member. Replace the binary and restart:

```sh
sudo install -m 0755 bin/tfrelay /usr/local/bin/tfrelay
sudo systemctl restart tfrelay
```

Sessions survive a relay restart, because they live on the members. Clients
reconnect and re-attach with their cursor, so they see only the gap.

Rebuild the client bundle before the binaries, always — `go:embed` bakes it in.

---

## The security model, stated plainly

**What the relay can see.** Everything. It terminates TLS, and its job is to
carry your keystrokes, so it reads your terminal output, your commands and your
files if you `cat` them. There is no end-to-end encryption here. The relay is a
machine you must own and trust, and it is the single most valuable host in this
deployment: compromising it compromises every machine that has enrolled.

**What protects it.**

- **TLS everywhere.** Members refuse a plain `ws://` relay unless told otherwise,
  and the relay refuses a non-loopback address without TLS.
- **A token that never touches a URL.** Browsers cannot set headers on a
  WebSocket, so the token is sent in the first frame. A query parameter would
  end up in Caddy's access log, in any intermediate proxy, and in browser
  history.
- **Rate-limited authentication.** Ten failures from one address inside a minute
  buys a five-minute lockout (`-auth-max-failures`). Per-address attribution uses
  the *last* `X-Forwarded-For` hop — the one the proxy appended — so a forged
  header does not let a caller invent a new identity per attempt.
- **One credential per machine.** A member presents its own secret, so revoking
  one machine does not disturb the rest.
- **A guarded admin panel.** The account is stored as a PBKDF2 hash (210k
  iterations, per-account salt), login attempts are throttled per address, the
  session cookie is HttpOnly + SameSite=Strict + Secure behind TLS, mutating
  requests are refused cross-origin, and the session store is in memory so a
  restart signs everyone out. It is the credential that mints every other one,
  so treat it like the root password of the fleet — and keep the whole
  deployment behind HTTPS, since the admin password crosses this connection.
- **The relay holds no root.** `deploy/tfrelay.service` runs with an empty
  capability set, a read-only filesystem, and no access to `/home`.

**What is not defended.** Anyone who has the client token can run commands on
every online machine — that is what the product is. Treat the token like an SSH
key, and prefer short-lived access at the proxy layer if you need more than one
person to reach it.

## Troubleshooting

**Members show as offline in the browser.** Check `tfrelay`'s `online` count
first. A member that cannot reach the relay logs a dial error and retries with
exponential backoff; the relay logs nothing, because no socket arrives.

**"访问令牌无效".** The token in the browser does not match
`TFRELAY_CLIENT_TOKEN`. The client clears it and asks again rather than retrying
forever — enter it again, or clear `termfleet.relay.token` from that browser's
localStorage.

**Everything reconnects every few minutes.** Something in the path is closing
idle WebSockets. The relay sends a ping and members answer; if a load balancer in
front has a shorter idle timeout, raise it above the ping interval.

**A session is missing after a restart.** Sessions belong to a member, so check
the member's own `/healthz` and its local session list. A restart of the relay
cannot take a session with it.
