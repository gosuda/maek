package tunnel

import (
	"context"
	"fmt"
	"io"

	"github.com/coder/websocket"
	"github.com/gosuda/maek/internal/protocol"
)

func writeControl(ctx context.Context, conn *websocket.Conn, typ string, payload any) error {
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

func readControl(ctx context.Context, conn *websocket.Conn) (protocol.Envelope, error) {
	_, r, err := conn.Reader(ctx)
	if err != nil {
		return protocol.Envelope{}, err
	}
	env, err := protocol.DecodeEnvelope(r)
	_, _ = io.Copy(io.Discard, r)
	return env, err
}

func WriteControl(ctx context.Context, conn *websocket.Conn, typ string, payload any) error {
	return writeControl(ctx, conn, typ, payload)
}

func ReadControl(ctx context.Context, conn *websocket.Conn) (protocol.Envelope, error) {
	return readControl(ctx, conn)
}

func negotiatedVersion(conn *websocket.Conn) (protocol.Version, error) {
	version, ok := protocol.ParseSubprotocol(conn.Subprotocol())
	if !ok {
		return 0, fmt.Errorf("server did not negotiate a supported maek subprotocol")
	}
	return version, nil
}

func ServerNegotiate(ctx context.Context, conn *websocket.Conn, softwareVersion string) (protocol.SessionConfig, error) {
	version, err := negotiatedVersion(conn)
	if err != nil {
		return protocol.SessionConfig{}, err
	}
	env, err := readControl(ctx, conn)
	if err != nil {
		return protocol.SessionConfig{}, err
	}
	if env.Type != protocol.MsgHello {
		return protocol.SessionConfig{}, fmt.Errorf("expected %q, got %q", protocol.MsgHello, env.Type)
	}
	hello, err := protocol.DecodePayload[protocol.Hello](env)
	if err != nil {
		return protocol.SessionConfig{}, err
	}
	encoding, ok := protocol.NegotiateEncoding(hello.Encodings, protocol.SupportedEncodings())
	if !ok {
		return protocol.SessionConfig{}, fmt.Errorf("no common content encoding")
	}
	capabilities := protocol.IntersectCapabilities(hello.Capabilities, protocol.SupportedCapabilities())
	welcome := protocol.Welcome{
		SoftwareVersion: softwareVersion,
		Encoding:        encoding,
		Capabilities:    capabilities,
	}
	if err := writeControl(ctx, conn, protocol.MsgWelcome, welcome); err != nil {
		return protocol.SessionConfig{}, err
	}
	return protocol.SessionConfig{Version: version, Encoding: encoding, Capabilities: capabilities}, nil
}

func ClientNegotiate(ctx context.Context, conn *websocket.Conn, softwareVersion string) (protocol.SessionConfig, error) {
	version, err := negotiatedVersion(conn)
	if err != nil {
		return protocol.SessionConfig{}, err
	}
	hello := protocol.Hello{
		SoftwareVersion: softwareVersion,
		Encodings:       protocol.SupportedEncodings(),
		Capabilities:    protocol.SupportedCapabilities(),
	}
	if err := writeControl(ctx, conn, protocol.MsgHello, hello); err != nil {
		return protocol.SessionConfig{}, err
	}
	env, err := readControl(ctx, conn)
	if err != nil {
		return protocol.SessionConfig{}, err
	}
	if env.Type == protocol.MsgErrorV1 {
		failure, decodeErr := protocol.DecodePayload[protocol.ProtocolError](env)
		if decodeErr != nil {
			return protocol.SessionConfig{}, decodeErr
		}
		return protocol.SessionConfig{}, fmt.Errorf("server rejected negotiation: %s (%s)", failure.Message, failure.Code)
	}
	if env.Type != protocol.MsgWelcome {
		return protocol.SessionConfig{}, fmt.Errorf("expected %q, got %q", protocol.MsgWelcome, env.Type)
	}
	welcome, err := protocol.DecodePayload[protocol.Welcome](env)
	if err != nil {
		return protocol.SessionConfig{}, err
	}
	if _, ok := protocol.NegotiateEncoding([]protocol.ContentEncoding{welcome.Encoding}, protocol.SupportedEncodings()); !ok {
		return protocol.SessionConfig{}, fmt.Errorf("server selected unsupported content encoding %q", welcome.Encoding)
	}
	capabilities := protocol.IntersectCapabilities(welcome.Capabilities, protocol.SupportedCapabilities())
	return protocol.SessionConfig{Version: version, Encoding: welcome.Encoding, Capabilities: capabilities}, nil
}
