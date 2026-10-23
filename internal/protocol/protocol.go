// Package protocol defines the wire format shared by tfd and every client.
//
// One WebSocket carries every session. Each frame is a single JSON object
// whose "t" field is the discriminator, so a client never needs a second
// connection no matter how many terminals it is watching.
package protocol

// Version is the protocol version carried by the hello handshake.
const Version = 2

// MaxMessageBytes bounds one WebSocket message, in bytes as they go on the
// wire: a JSON object with its Data base64ed, and for a frame travelling
// through the relay, base64ed once more inside the tunnel frame.
//
// It exists because coder/websocket has a default read limit of 32768 bytes and
// a frame over it does not arrive clipped — the socket dies with
// StatusMessageTooBig, which a reconnecting client reads as "the machine closed
// the stream". The bound has to be generous enough for a whole 256 KiB ring
// replay (see internal/session.ringCapacity), which in a tunnel frame is
// roughly 466 KiB, while still leaving a hard ceiling that stops a wedged or
// malicious peer from making the process allocate without limit.
const MaxMessageBytes = 1 << 20

// MaxOutputChunk is how many bytes of PTY output one output frame may carry
// before the hub splits the rest into further frames.
//
// It is deliberately far below MaxMessageBytes: peers older than this constant
// — a relay or a desktop build from before the read limits were raised — still
// refuse anything over 32768 bytes, and a replay that fits in one small frame
// is accepted by them too. 16 KiB of output becomes about 22 KiB of JSON and
// about 29 KiB inside a tunnel frame, which clears the old default with room
// for the field names around it.
const MaxOutputChunk = 16 * 1024

// Client -> server discriminators.
const (
	CTHello   = "hello"
	CTList    = "list"
	CTCreate  = "create"
	CTAttach  = "attach"
	CTDetach  = "detach"
	CTInput   = "input"
	CTResize  = "resize"
	CTClaim   = "claim"
	CTRelease = "release"
	CTClose   = "close"
	CTRename  = "rename"
)

// Server -> client discriminators. STHello deliberately shares the string
// "hello" with CTHello; the direction of travel disambiguates them.
const (
	STHello    = "hello"
	STSessions = "sessions"
	STCreated  = "created"
	STAttached = "attached"
	STClosed   = "closed"
	STExit     = "exit"
	STOutput   = "output"
	STRole     = "role"
	STError    = "error"
)

// Roles. A session has at most one controller; everyone else is a viewer.
const (
	RoleController = "controller"
	RoleViewer     = "viewer"
)

// Error codes. These are stable identifiers, not prose: clients branch on
// them, and only Message is meant to be shown to a person.
const (
	ErrProtocol      = "protocol"
	ErrBadRequest    = "bad_request"
	ErrNoSession     = "no_session"
	ErrNotController = "not_controller"
	ErrBadSize       = "bad_size"
	ErrUnauthorized  = "unauthorized"
	// ErrLeaseHeld is what a claim gets when another client holds the lease
	// and it has not lapsed. The holder is named in Message, and the claim
	// may be repeated with Steal to take it anyway.
	ErrLeaseHeld = "lease_held"
	// ErrControlOff is a claim, an attach that would have taken an unclaimed
	// shell, or a create refused because the machine does not allow other
	// clients to control it. It is an answer rather than a failure: watching
	// still works, and the machine's own window is never refused.
	ErrControlOff = "control_off"
)

// Msg is one frame. Every field is optional; T decides which are meaningful.
//
// Data is []byte so encoding/json base64s it. PTY output is not guaranteed to
// be valid UTF-8, and plain string fields would have the invalid bytes
// silently rewritten to U+FFFD.
type Msg struct {
	T string `json:"t"`

	// Handshake and listing.
	V      int    `json:"v,omitempty"`
	ReqID  string `json:"reqId,omitempty"`
	Host   *Host  `json:"host,omitempty"`
	Reason string `json:"reason,omitempty"`

	// Session addressing.
	SessionID string    `json:"sessionId,omitempty"`
	Session   *Summary  `json:"session,omitempty"`
	Sessions  []Summary `json:"sessions,omitempty"`

	// create
	Cols  int    `json:"cols,omitempty"`
	Rows  int    `json:"rows,omitempty"`
	Cwd   string `json:"cwd,omitempty"`
	Shell string `json:"shell,omitempty"`
	Title string `json:"title,omitempty"`
	// From names a live session of this machine's whose working directory the
	// new shell opens in — the directory that shell is in *now*, not the one
	// it was started in, so a split or a new tab continues where the shell
	// beside it stands rather than at the home directory. A session that is
	// gone, or a machine that cannot say where its processes stand, falls
	// back to Cwd and then to the home directory.
	From string `json:"from,omitempty"`

	// attach / output. Cursor is a byte offset into the session's output
	// stream; see internal/session.Ring.
	Cursor    int64  `json:"cursor,omitempty"`
	Data      []byte `json:"data,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Resync    bool   `json:"resync,omitempty"`

	// role
	Role       string `json:"role,omitempty"`
	Controller string `json:"controller,omitempty"`

	// The lease. Epoch counts every change of who holds a session's lease: the
	// role frame tells every client the epoch in force, and input, resize and
	// close carry the epoch they were sent under. A frame whose epoch is not
	// the current one belonged to a holder that has since been replaced, and
	// the owner drops it, so input from a controller that was just revoked
	// cannot land after the handoff.
	Epoch uint64 `json:"epoch,omitempty"`

	// claim. Steal takes the lease from a holder whose lease has not lapsed;
	// without it such a claim is refused with ErrLeaseHeld. The machine the
	// PTY runs on never needs it: its own window's claim is always honoured,
	// because it is the machine's shell.
	Steal bool `json:"steal,omitempty"`

	// error
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`

	// exit
	ExitCode int `json:"exitCode,omitempty"`

	// Dismiss is what the hub repeats on the exit frame it broadcasts: the tab
	// goes with the process, so every client drops its own copy instead of
	// keeping it to read the last lines. A close always dismisses unless it asks
	// to keep the tab — see Keep.
	Dismiss bool `json:"dismiss,omitempty"`

	// Keep is what 结束 adds to the close frame: end the process, but leave the
	// tab on every client so its final output can be read. Closing a tab is the
	// default a close gets without it, because a close means a close in a
	// terminal — and because a client older than this field sends the same
	// frame for both acts, and leaving dead tabs on every other screen is the
	// worse of the two guesses.
	Keep bool `json:"keep,omitempty"`
}

// Host identifies the machine a daemon runs on. It is already in the
// handshake so that a future hub can group sessions by machine without a
// second protocol.
type Host struct {
	Hostname string `json:"hostname"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
}

// Summary is one row of the session list. Clients render exactly one row per
// live session and keep the order the server sent — a client may keep an order
// of its own beside it, but the list's order is the one every screen shares.
type Summary struct {
	SessionID string `json:"sessionId"`
	PID       int    `json:"pid"`
	Cwd       string `json:"cwd"`
	// Title is what the row is called: the name a rename gave the shell, else
	// the title the program set on its terminal (OSC 0/2) — which is how an
	// agent names the task it is on. Empty leaves the clients their own
	// fallback.
	Title string `json:"title"`
	// Shell is the program the session runs. Clients fall back to it when the
	// shell has not reported a title yet, so a fresh tab reads as "zsh 2"
	// rather than a bare session id.
	Shell      string   `json:"shell,omitempty"`
	Cols       int      `json:"cols"`
	Rows       int      `json:"rows"`
	Alive      bool     `json:"alive"`
	Cursor     int64    `json:"cursor"`
	Controller string   `json:"controller,omitempty"`
	Viewers    []string `json:"viewers,omitempty"`
	CreatedAt  string   `json:"createdAt"`

	// What the shell is doing, which the daemon follows from the session's own
	// output: see internal/status. Status is one of idle, working, waiting,
	// error or exited; Unread marks a state the user has not looked at yet;
	// Message is what the program said about it; Kind says what a waiting
	// program wants (permission, question, auth); Progress is 0-100 while a
	// working program reports one, -1 otherwise; Since is when the state began,
	// in Unix milliseconds.
	Status   string `json:"status,omitempty"`
	Unread   bool   `json:"unread,omitempty"`
	Message  string `json:"message,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Progress int    `json:"progress,omitempty"`
	Since    int64  `json:"since,omitempty"`
	// Process is the program in the foreground of the pane — "zsh" at a prompt,
	// "claude" while an agent runs — which the daemon reads from the terminal's
	// own process group. Empty on a host that cannot say.
	Process string `json:"process,omitempty"`
}
