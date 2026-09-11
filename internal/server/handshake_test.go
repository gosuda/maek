package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gosuda/maek/internal/protocol"
	"github.com/gosuda/maek/internal/tunnel"
)

func TestAgentWebSocketV2MultiServiceHandshake(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + httpSrv.URL[len("http"):] + protocol.EndpointWS
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{Subprotocols: protocol.SupportedSubprotocols()})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if err := tunnel.ValidateSubprotocol(conn); err != nil {
		t.Fatal(err)
	}

	request := protocol.RegisterServices{
		SoftwareVersion: "test",
		Services: []protocol.ServiceSpec{
			{Alias: "shared"},
			{Alias: "shared"},
		},
	}
	if err := tunnel.WriteControl(ctx, conn, protocol.MsgRegister, request); err != nil {
		t.Fatal(err)
	}
	env, err := tunnel.ReadControl(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := protocol.DecodePayload[protocol.RegisteredServices](env)
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != protocol.MsgRegistered || len(registered.Services) != 2 {
		t.Fatalf("unexpected response: %+v", registered)
	}
	if registered.Services[0].ID != "shared" || registered.Services[1].ID != "shared-2" {
		t.Fatalf("unexpected alias-derived IDs: %+v", registered.Services)
	}

	tunnelSession, err := tunnel.NewClient(conn)
	if err != nil {
		t.Fatal(err)
	}
	defer tunnelSession.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.Registry().List()) == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := srv.Registry().List(); len(got) != 2 {
		t.Fatalf("got %d registered services", len(got))
	}
	entry, ok := srv.Registry().Get("shared")
	if !ok || entry.Info.ID != registered.Services[0].ID {
		t.Fatalf("oldest alias owner not selected: %+v", entry)
	}
}
