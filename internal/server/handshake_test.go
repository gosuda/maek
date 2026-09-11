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

func TestAgentWebSocketV1MultiServiceHandshake(t *testing.T) {
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

	config, err := tunnel.ClientNegotiate(ctx, conn, "test")
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != protocol.Version1 {
		t.Fatalf("got version %d", config.Version)
	}

	request := protocol.RegisterServices{Services: []protocol.ServiceSpec{
		{PreferredID: "owner-a", Alias: "shared"},
		{PreferredID: "owner-b", Alias: "shared"},
	}}
	if err := tunnel.WriteControl(ctx, conn, protocol.MsgRegisterV1, request); err != nil {
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
	if err := tunnel.WriteControl(ctx, conn, protocol.MsgStartV1, struct{}{}); err != nil {
		t.Fatal(err)
	}

	netConn := websocket.NetConn(ctx, conn, websocket.MessageBinary)
	tunnelSession, err := tunnel.NewClient(netConn, config)
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
	if !ok || entry.Info.ID != "owner-a" {
		t.Fatalf("oldest alias owner not selected: %+v", entry)
	}
}
