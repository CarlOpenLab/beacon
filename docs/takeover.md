# Taking over a shell on another machine

The desktop already speaks to a local daemon and uses one controller per
session (`claim`/`release` in the protocol). This is the design for letting a
desktop that is registered on a relay reach *other* machines, take one of their
shells, and hand it back without two people typing into the same shell.

**Status: built.** The lease, the reach and the chrome are in. What is left is
the access-control layer, at the bottom of this file.

## One writer, and it is the machine holding the PTY

* The daemon holding the PTY is the owner: the only writer to the PTY, and the
  only sequencer of input. The relay carries frames and never writes.
* A session has a **lease**: `{holder, epoch, deadline}`. Attaching without the
  lease is read-only. `claim` takes it, `release` gives it back, and taking it
  from someone else asks first (`steal` → a toast on their side, forced only
  after the lease deadline or with an explicit confirmation).
* Every input frame carries the epoch it was sent under. The owner drops frames
  whose epoch is stale, so input from a controller that was just revoked cannot
  land after the handoff.
* The lease holder acks the sequence it consumed; viewers render the PTY's own
  output, so every window shows the same bytes.

## What a taken-over shell looks like

The lease is already broadcast to every client, so the "someone else has this"
state needs no new channel:

* A shell whose holder is not this window paints a soft animated glow around its
  pane (the `examples/chat-glow` look) plus a popover naming who holds it, with
  **拿回控制权** / stay read-only. The tab's row in the column carries the
  holder's name too, and the header of the shell on screen carries the holder and
  the same offer, so the way back is where the typing is. A shell taken from this
  window opens that popover by itself, and so does typing into a shell this
  window does not hold — the hub answers a refused keystroke loudly, and the
  window says it where the typing went.
* Reclaiming is local: the machine with the PTY releases the lease with its own
  epoch bump, and the remote controller drops to read-only in the same frame.
  The machine's own window never needs a confirmation to take its shell back —
  the hub marks that client as the owner and honours its claim over anyone
  else's.
* The effect is the ring shader from that example, compiled by MyGo for whatever
  draws the window and interpreted on the CPU where nothing can: it runs only
  while someone else holds the lease, it is drawn along the pane's edge rather
  than over the text (no re-layout, no re-shaping of the grid), its fade is an
  animation so a desktop asking for less motion gets it at once, and its colour
  flow stands still for the same reason.

## Latency: two different problems

1. **Two people typing at once** — solved by the lease, and latency plays no
   part in it. This is the part worth engineering carefully.
2. **Typing through a laggy link** — cannot be removed, only hidden. The rules:

* Never echo keystrokes locally on a remote shell. The PTY's echo is the only
  truth; a local echo is exactly how the two sides start disagreeing.
* Batch keys into one frame per frame time (~16 ms) and sequence them, so a slow
  link sends fewer, larger frames and a re-ordered frame is dropped, never
  applied twice.
* Show the truth: an input line is "sent" when the owner acks it, not when it is
  typed — a pending span that greys out until the ack is honest about the round
  trip without lying about the state.
* Freeze during handoff: while a `lease` change is in flight, input is held and
  the popover says 接管中. After the epoch bump the old controller's queue is
  dropped, so nothing interleaves mid-transition.
* Expire a silent lease: no ack for N seconds and control returns to the machine
  in front of the PTY, so a controller whose network died cannot lock someone
  out of their own shell.
* Resume instead of resending: a viewer keeps a short replay buffer plus the last
  sequence it consumed and asks for the missing tail on reconnect (the same
  session resume the browser client already uses).

## How it is wired

* **Presence**: the relay already broadcasts its roster to clients
  (`machines`, pushed on every presence change). The desktop window opens a
  *client* connection beside its member one — a second link with the fleet's
  client token rather than this machine's device credential — so it receives
  that roster and lists it in the 机器 panel. No relay change was needed.
* **Remote attach**: `internal/relayclient` is the Go half of the relay protocol
  for a program that drives a fleet. One stream per machine carries the session
  protocol inside it, so the window is an ordinary client of that machine's hub
  — the same frames and the same arbitration as a browser, over the tunnel. The
  window's local link is the same `link` over an in-process pipe
  (`hub.Pipe`, `ServeLocal`), which is how the machine's own window is a client
  of its own hub and can be marked the owner.
* **Access control**: what is in place today is the lease itself, the owner's
  right to its own shell, and the machine's own switch over who may control it.
  What is still missing:

* **Per-device authorization**: the relay knows a client by the fleet's shared
  token, so any client that holds it may drive any enrolled machine that allows
  being driven. A machine that wants to say "this laptop, not that one" needs a
  per-device ACL on the relay, which does not exist yet. Until then, the client
  token is the whole authority: treat it like the fleet's password, because it
  is.
* **Consent, and why it is not a dialog**: the design first asked the holder to
  approve a steal, the way a screen share does. That was dropped: a fleet is one
  person's machines, and nobody is sitting at the far machine to answer. Being
  enrolled with a relay *is* the consent, and the honest version of "ask them
  first" turned out to be a switch on the machine rather than a question to the
  person — 服务 → 允许被控制, and `tfd -allow-control=false` for a headless
  member. Off, other clients watch and nothing more: no lease, no new shells,
  and a holder that is not the machine's own window loses the shell in the frame
  the switch moves. The machine's own window is never refused, because a switch
  on a machine must not lock the machine out of itself.