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

* A shell whose holder is not this window is marked where the shell is: a 只读
  chip in the corner of its pane, and the pane keeps its own colour rather than
  gaining a second one. The header of the shell on screen names the holder and
  offers **拿回控制权** / stay read-only, the tab's row in the column carries the
  holder's name too, and the composer says 只读：等待接管 where a sendable one
  says what Enter does — so the way back is where the typing is, stated in the
  same words the browser client uses. A shell taken from this window opens that
  offer by itself, and so does typing into a shell this window does not hold —
  the hub answers a refused keystroke loudly, and the window says it where the
  typing went. The offer says what pressing it does to the other side — *c9
  会变成只读，之后仍能看着这个终端。* — because a button that takes a keyboard
  away from somebody without saying so is one people learn not to press.
* A shell **nobody** holds is marked read-only too. It is a different fact — no
  one is typing — but it is the same answer at the keyboard, so the pane keeps
  its 只读 chip, the band says 只读：当前无人控制 with the way in beside it, and the
  row in the panel says 无人控制 rather than 控制中. A shell on another machine
  lands there whenever its lease is let go with the machine that owns the PTY
  not watching, and the hub's own refusal has to tell it apart from a shell
  somebody has: *nobody holds s1; claim it to type* is a sentence with no second
  person in it, and the other one sends a person looking for one.
* One of those marks is not words: a light along the pane's edge, and it is
  drawn under exactly one condition — a shell **this machine runs**, driven by
  another client. Somebody sitting at their own desk reads it from across the
  room as "somebody else has my keyboard", which is the one thing none of the
  words on screen say at that distance. It is deliberately absent on a shell of
  another machine's: nobody here owns that process, and a light that came on for
  every shell anyone held would come on for the ordinary case, which is how a
  signal stops being one. A shell this machine runs and this window drives has
  no light either — the lease is where it belongs.
* It is a glow and not a border. The band is the accent's own three tones
  drifting slowly round the edge, with no line anywhere in it and the light
  falling off on both sides of the pane's edge, so the pane reads as lit rather
  than as outlined — an outline reads as something the terminal has become, and
  the terminal has not changed. Where the desktop asks for less motion the
  colours simply stop travelling: the light is the state and the drift is not,
  so the still window is the same window, stopped.
* Reclaiming is local: the machine with the PTY releases the lease with its own
  epoch bump, and the remote controller drops to read-only in the same frame.
  The machine's own window never needs a confirmation to take its shell back —
  the hub marks that client as the owner and honours its claim over anyone
  else's.
* Handing a shell back is offered for a shell this window is driving *somewhere
  else*: the lease returns to the machine the shell runs on, which is told in the
  same frame and can type again. A shell of this machine's is never offered the
  release, and not because it is rare: the machine that owns the PTY has nobody
  to hand its own lease to, so releasing it leaves the lease held by no one — and
  the one thing the button would do is stop the window in front of the shell
  taking the typing of the person sitting at it.

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
* A dropped link is a routing outage, never a dead pane: the browser client
  re-attaches every tab's session at its cursor when its link comes back, and
  the desktop window does the same for its tabs — a tab whose relay stream ended
  is re-watched on the machine's link as it now stands, either when the row is
  clicked again or when the machine is looked at again, and the hub's replay
  from byte 0 paints the shell's screen onto a fresh terminal. A tab that stays
  blank forever was the failure this replaced.

## How it is wired

* **Presence**: the relay already broadcasts its roster to clients
  (`machines`, pushed on every presence change). The desktop window opens a
  *client* connection beside its member one, so it receives that roster and lists
  it in the 机器 panel — a list you switch between, this machine first, with the
  relay itself getting no row: it is the pipe, not a machine.
* **The credential a window drives with**: not the fleet's shared secret. An
  enrolled machine asks its relay for a short-lived *control token*
  (`POST /relay/control-token`, authenticated with the device credential it
  already dials the member link with), and the window's client link presents
  that. So reaching the rest of the fleet is something a machine does rather than
  something a person types, a leaked control token expires on its own in minutes,
  and revoking the machine at the relay takes the reach away at once
  (`Relay.RevokeDevice` forgets the tokens it minted). The shared client token
  stays what it was: what a browser or a phone is given.
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

* **Per-device authorization**: any *enrolled* machine may drive any other that
  allows being driven — that is the intended shape for one person's fleet, and
  the switch below is the brake — but nothing finer exists yet. A machine that
  wants to say "this laptop, not that one" needs an ACL on the relay: who may
  mint a control token for which machine. Until then, the device credential is
  the whole authority for control: whoever holds a machine's secret can drive the
  fleet from it. The shared token, meanwhile, only ever buys the roster and the
  read-only view a viewer gets.
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