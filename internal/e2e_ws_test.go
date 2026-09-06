package internal_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gosuda/maek/internal/agent"
	"github.com/gosuda/maek/internal/protocol"
	"github.com/gosuda/maek/internal/server"
)

func TestE2E_WebSocketProxy(t *testing.T) {
	// 1. Target server with WebSocket echo
	targetMux := http.NewServeMux()
	targetMux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			t.Logf("target accept error: %v", err)
			return
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		ctx := r.Context()
		for {
			typ, msg, err := c.Read(ctx)
			if err != nil {
				return
			}
			err = c.Write(ctx, typ, msg)
			if err != nil {
				return
			}
		}
	})
	targetServer := httptest.NewServer(targetMux)
	defer targetServer.Close()

	// 2. Start maek server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	serverAddr := listener.Addr().String()

	srv := server.NewServer(server.Config{Addr: serverAddr})
	httpSrv := &http.Server{Handler: srv.Handler()}
	go func() {
		_ = httpSrv.Serve(listener)
	}()
	defer httpSrv.Close()

	// 3. Start maek agent
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ag, err := agent.NewAgent(agent.Config{
		ServerURL: "ws://" + serverAddr,
		Name:      "ws-service",
		Target:    targetServer.URL,
	})
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	go func() {
		_ = ag.Start(ctx)
	}()

	// Wait for agent to register
	var serviceID string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		services := srv.Registry().List()
		if len(services) > 0 {
			serviceID = services[0].ID
			break
		}
	}
	if serviceID == "" {
		t.Fatalf("agent did not register")
	}

	// 4. Connect WebSocket client through maek server using Header routing
	wsURL := "ws://" + serverAddr + "/ws"
	dialOpts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			protocol.HeaderService: []string{serviceID},
		},
	}

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dialCancel()

	clientConn, resp, err := websocket.Dial(dialCtx, wsURL, dialOpts)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("failed to connect websocket through maek (status: %d): %v", status, err)
	}
	defer clientConn.Close(websocket.StatusNormalClosure, "")

	// 5. Echo test
	testMsg := "hello websocket through maek"
	err = clientConn.Write(dialCtx, websocket.MessageText, []byte(testMsg))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_, readMsg, err := clientConn.Read(dialCtx)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if string(readMsg) != testMsg {
		t.Fatalf("got %q, want %q", string(readMsg), testMsg)
	}
}
