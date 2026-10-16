# Taking over a shell on another machine

The desktop already speaks to a local daemon and uses one controller per
session (`claim`/`release` in the protocol). This is the design for letting a
desktop that is registered on a relay reach *other* machines, take one of their
shells, and hand it back without two people typing into the same shell.

## One writer, and it is the machine holding the PTY

* The daemon holding the PTY is the owner: the only writer to the PTY, and the
  only sequencer of input. The relay carries frames and never writes.
* A session has a **lease**: `{holder, epoch, deadline}`. Attaching without the
  lease is read-only. `claim` takes it, `release` gives it back, and taking it
  from someone else asks them first (`steal` → a toast on their side, forced
  only after the lease deadline or with an explicit confirmation).
* Every input frame carries the epoch it was sent under. The owner drops frames
  whose epoch is stale, so input from a controller that was just revoked cannot
  land after the handoff.
* The lease holder acks the sequence it consumed; viewers render the PTY's own
  output, so every window shows the same bytes.

## What a taken-over shell looks like

The lease is already broadcast to every client, so the "someone else has this"
state needs no new channel:

* A shell whose holder is not this window paints a soft animated glow behind its
  text (the `examples/chat-glow` look) plus a floating popover over the terminal:
  who is attached, and **拿回控制权** / stay read-only.
* Reclaiming is local: the machine with the PTY releases the lease with its own
  epoch bump, and the remote controller drops to read-only in the same frame.
* Keep the effect cheap and honest: it runs only while someone else holds the
  lease, it is drawn behind the grid (no re-layout, no re-shaping of text), and
  it is skipped when reduced motion is set.

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

## What is missing today

* **Presence**: the relay has no device registry. Add `device.announce`
  (id, name, sessions) and a `fleet.devices` broadcast; the desktop rail gets a
  机器 section listing other machines and their shells.
* **Access control**: taking over must be authorized per device pair (the relay
  knows clients by join secret; extend it with a per-device ACL and a visible
  first-time confirmation on the target machine).
* **Remote attach in the desktop**: the native client speaks the local hub
  protocol; it needs the relay's device route, which `internal/viewer` and the
  web client already model.