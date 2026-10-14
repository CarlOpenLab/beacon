// Package protocol defines the wire format shared by tfd and every client.
//
// One WebSocket carries every session. Each frame is a single JSON object
// whose "t" field is the discriminator, so a client never needs a second
// connection no matter how many terminals it is watching.
package protocol

// Version is the protocol version carried by the hello handshake.
const Version = 1

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

	// attach / output. Cursor is a byte offset into the session's output
	// stream; see internal/session.Ring.
	Cursor    int64  `json:"cursor,omitempty"`
	Data      []byte `json:"data,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Resync    bool   `json:"resync,omitempty"`

	// role
	Role       string `json:"role,omitempty"`
	Controller string `json:"controller,omitempty"`

	// error
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`

	// exit
	ExitCode int `json:"exitCode,omitempty"`
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
// live session and keep the order the server sent, which is what makes the
// phone list and the PC list one-to-one.
type Summary struct {
	SessionID  string   `json:"sessionId"`
	PID        int      `json:"pid"`
	Cwd        string   `json:"cwd"`
	Title      string   `json:"title"`
	Cols       int      `json:"cols"`
	Rows       int      `json:"rows"`
	Alive      bool     `json:"alive"`
	Cursor     int64    `json:"cursor"`
	Controller string   `json:"controller,omitempty"`
	Viewers    []string `json:"viewers,omitempty"`
	CreatedAt  string   `json:"createdAt"`
}
