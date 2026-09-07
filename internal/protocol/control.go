package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/coder/websocket"
)

// Control message types exchanged over the agent WebSocket before the
// yamux data plane starts.
const (
	MsgTypeRegister   = "register"   // agent -> server: register a service
	MsgTypeRegistered = "registered" // server -> agent: service accepted, carries assigned ID
	MsgTypeStart      = "start"      // agent -> server: registration phase done, switch to data plane
	MsgTypeError      = "error"      // server -> agent: handshake failed
)

// RegisterRequest carries service metadata inside a "register" message.
type RegisterRequest struct {
	Name        string `json:"name,omitempty"`
	ID          string `json:"id,omitempty"` // preferred service ID
	Description string `json:"desc,omitempty"`
	Thumbnail   string `json:"thumb,omitempty"`
}

// AgentMessage is the envelope for agent <-> server control messages.
//
// The handshake is designed as a state machine that accepts one or more
// "register" messages followed by a single "start". Today the server
// enforces exactly one "register" per connection (one app per WebSocket),
// but the framing itself leaves room for multi-service connections: extra
// registrations can later be carried by a control stream inside yamux
// using the same message shapes.
type AgentMessage struct {
	Type    string           `json:"type"`
	Service *RegisterRequest `json:"service,omitempty"` // for "register"
	ID      string           `json:"id,omitempty"`      // for "registered"
	Name    string           `json:"name,omitempty"`    // for "registered"
	Code    string           `json:"code,omitempty"`    // for "error"
	Message string           `json:"message,omitempty"` // for "error"
}

// WriteAgentMessage sends one control message as a text frame.
func WriteAgentMessage(ctx context.Context, c *websocket.Conn, m AgentMessage) error {
	w, err := c.Writer(ctx, websocket.MessageText)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(w).Encode(m); err != nil {
		return err
	}
	return w.Close()
}

// ReadAgentMessage reads one control message, draining the rest of the
// frame so subsequent reads on the connection stay valid.
func ReadAgentMessage(ctx context.Context, c *websocket.Conn) (AgentMessage, error) {
	var m AgentMessage
	_, r, err := c.Reader(ctx)
	if err != nil {
		return m, err
	}
	dec := json.NewDecoder(r)
	if err := dec.Decode(&m); err != nil {
		return m, fmt.Errorf("invalid control message: %w", err)
	}
	_, _ = io.Copy(io.Discard, r)
	return m, nil
}
