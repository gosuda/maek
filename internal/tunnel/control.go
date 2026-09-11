package tunnel

import (
	"context"
	"fmt"
	"io"

	"github.com/coder/websocket"
	"github.com/gosuda/maek/internal/protocol"
)

func WriteControl(ctx context.Context, conn *websocket.Conn, typ string, payload any) error {
	w, err := conn.Writer(ctx, websocket.MessageText)
	if err != nil {
		return err
	}
	if err := protocol.EncodeEnvelope(w, typ, payload); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func ReadControl(ctx context.Context, conn *websocket.Conn) (protocol.Envelope, error) {
	typ, r, err := conn.Reader(ctx)
	if err != nil {
		return protocol.Envelope{}, err
	}
	if typ != websocket.MessageText {
		_, _ = io.Copy(io.Discard, r)
		return protocol.Envelope{}, fmt.Errorf("expected text control message")
	}
	env, err := protocol.DecodeEnvelope(r)
	_, _ = io.Copy(io.Discard, r)
	return env, err
}

func ValidateSubprotocol(conn *websocket.Conn) error {
	version, ok := protocol.ParseSubprotocol(conn.Subprotocol())
	if !ok || version != protocol.Version2 {
		return fmt.Errorf("expected %q websocket subprotocol, got %q", protocol.Version2.Subprotocol(), conn.Subprotocol())
	}
	return nil
}
