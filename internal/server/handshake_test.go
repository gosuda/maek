package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gosuda/maek/internal/protocol"
)

func dialAgentWS(t *testing.T, h http.Handler, path string) *websocket.Conn {
	t.Helper()
	httpSrv := httptest.NewServer(h)
	t.Cleanup(httpSrv.Close)

	wsURL := "ws://" + httpSrv.Listener.Addr().String() + path
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("agent websocket dial failed: %v", err)
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

func waitForService(t *testing.T, srv *Server, id string) protocol.ServiceInfo {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, svc := range srv.Registry().List() {
			if svc.ID == id {
				return svc
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("service %q did not register within deadline", id)
	return protocol.ServiceInfo{}
}

func TestAgentWebSocketFrameHandshake(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	c := dialAgentWS(t, srv.Handler(), protocol.EndpointWS)
	ctx := context.Background()

	// 1. register with metadata
	if err := protocol.WriteAgentMessage(ctx, c, protocol.AgentMessage{
		Type: protocol.MsgTypeRegister,
		Service: &protocol.RegisterRequest{
			Name:        "frame-app",
			ID:          "frame-app",
			Description: "via control frames",
		},
	}); err != nil {
		t.Fatalf("register send failed: %v", err)
	}

	ack, err := protocol.ReadAgentMessage(ctx, c)
	if err != nil {
		t.Fatalf("registered read failed: %v", err)
	}
	if ack.Type != protocol.MsgTypeRegistered || ack.ID != "frame-app" {
		t.Fatalf("expected registered/frame-app, got type=%q id=%q", ack.Type, ack.ID)
	}

	// 2. start the data plane
	if err := protocol.WriteAgentMessage(ctx, c, protocol.AgentMessage{
		Type: protocol.MsgTypeStart,
	}); err != nil {
		t.Fatalf("start send failed: %v", err)
	}

	// 3. service must appear with the registered metadata
	svc := waitForService(t, srv, "frame-app")
	if svc.Name != "frame-app" || svc.Description != "via control frames" {
		t.Errorf("unexpected service info: %+v", svc)
	}
}

func TestAgentWebSocketFrameHandshakeRejectsSecondRegister(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	c := dialAgentWS(t, srv.Handler(), protocol.EndpointWS)
	ctx := context.Background()

	register := protocol.AgentMessage{
		Type:    protocol.MsgTypeRegister,
		Service: &protocol.RegisterRequest{Name: "first", ID: "first"},
	}
	if err := protocol.WriteAgentMessage(ctx, c, register); err != nil {
		t.Fatalf("register send failed: %v", err)
	}
	if ack, err := protocol.ReadAgentMessage(ctx, c); err != nil || ack.Type != protocol.MsgTypeRegistered {
		t.Fatalf("expected registered, got %+v (%v)", ack, err)
	}

	register.Service = &protocol.RegisterRequest{Name: "second", ID: "second"}
	if err := protocol.WriteAgentMessage(ctx, c, register); err != nil {
		t.Fatalf("second register send failed: %v", err)
	}

	msg, err := protocol.ReadAgentMessage(ctx, c)
	if err != nil {
		t.Fatalf("expected error frame, got read error: %v", err)
	}
	if msg.Type != protocol.MsgTypeError {
		t.Fatalf("expected error frame, got %+v", msg)
	}
}

func TestAgentWebSocketLegacyQueryFallback(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	// Legacy agents pass metadata via query params and go straight to yamux.
	c := dialAgentWS(t, srv.Handler(), protocol.EndpointWS+"?name=legacy-app&id=legacy-app")

	svc := waitForService(t, srv, "legacy-app")
	if svc.Name != "legacy-app" {
		t.Errorf("unexpected service info: %+v", svc)
	}
	_ = c.Close(websocket.StatusNormalClosure, "")
}
