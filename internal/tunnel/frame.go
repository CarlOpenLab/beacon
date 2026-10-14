// Package tunnel is the wire format between a relay and the things that talk
// to it: fleet members (which run a hub) and clients (which drive one).
//
// It is deliberately thin. The relay routes streams and never looks inside
// Payload, so the session protocol in internal/protocol rides through it
// untouched — a feature added to the hub tomorrow needs no relay change.
//
// Both directions of a stream carry the same v1 frames a LAN client would
// send straight to a daemon; only the addressing differs.
package tunnel

import "github.com/CarlOpenLab/term-fleet/internal/protocol"

// Version is the tunnel protocol version, carried by agent.hello.
const Version = 1

// Discriminators.
const (
	// Member -> relay.
	TAgentHello   = "agent.hello"
	TAgentWelcome = "agent.welcome" // relay -> member

	// Client -> relay.
	TMachines = "machines" // relay -> client; also pushed on presence change
	TOpen     = "open"     // client -> relay -> member
	TAccept   = "accept"   // relay -> client, sent when Open is routed
	TData     = "data"     // both directions
	TClose    = "close"    // both directions
	TError    = "error"
)

// Error codes. Stable identifiers; clients branch on them.
const (
	ErrUnauthorized   = "unauthorized"
	ErrBadRequest     = "bad_request"
	ErrNoMachine      = "no_machine"
	ErrMachineOffline = "machine_offline"
	ErrStreamLimit    = "stream_limit"
)

// Frame is one tunnel message.
//
// Stream is scoped to the connection it is sent on, not to the machine: the
// relay hands each client stream its own member-side number and translates, so
// neither peer has to reason about another peer's identifiers.
type Frame struct {
	T string `json:"t"`
	V int    `json:"v,omitempty"`

	// Routing (open/data/close).
	Machine string `json:"machine,omitempty"`
	Stream  uint32 `json:"stream,omitempty"`

	// The inner protocol.Msg, opaque to the relay. []byte so encoding/json
	// base64s it — the same reason protocol.Msg.Data is []byte.
	Payload []byte `json:"payload,omitempty"`

	// Registry (machines, agent.hello).
	Machines    []Machine     `json:"machines,omitempty"`
	MachineID   string        `json:"machineId,omitempty"`
	MachineName string        `json:"machineName,omitempty"`
	Host        protocol.Host `json:"host,omitempty"`

	// error
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Machine is one fleet member as the relay knows it. Online flips as members
// come and go; the relay pushes a fresh list every time it does.
type Machine struct {
	ID     string        `json:"id"`
	Name   string        `json:"name,omitempty"`
	Host   protocol.Host `json:"host"`
	Online bool          `json:"online"`
}
