package tunnel

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func websocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	type accepted struct {
		conn *websocket.Conn
		err  error
	}
	acceptedCh := make(chan accepted, 1)
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		acceptedCh <- accepted{conn: conn, err: err}
		if err == nil {
			<-done
		}
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	client, _, err := websocket.Dial(ctx, "ws"+srv.URL[len("http"):], nil)
	cancel()
	if err != nil {
		close(done)
		srv.Close()
		t.Fatal(err)
	}
	result := <-acceptedCh
	if result.err != nil {
		client.CloseNow()
		close(done)
		srv.Close()
		t.Fatal(result.err)
	}

	t.Cleanup(func() {
		client.CloseNow()
		result.conn.CloseNow()
		close(done)
		srv.Close()
	})
	return result.conn, client
}

func sessionPair(t *testing.T, policy FlowPolicy) (*Session, *Session) {
	t.Helper()
	serverWS, clientWS := websocketPair(t)
	server, err := newSession(serverWS, roleServer, policy)
	if err != nil {
		t.Fatal(err)
	}
	client, err := newSession(clientWS, roleClient, policy)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		server.Close()
		client.Close()
	})
	return server, client
}

func openPair(t *testing.T, server, client *Session, serviceID string) (net.Conn, net.Conn) {
	t.Helper()
	acceptedCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := client.Listener().Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptedCh <- conn
	}()

	serverConn, err := server.OpenHTTP(serviceID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		t.Fatal(err)
	case clientConn := <-acceptedCh:
		return serverConn, clientConn
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream accept")
	}
	return nil, nil
}

func TestStreamRoundTripAndServiceID(t *testing.T) {
	server, client := sessionPair(t, DefaultFlowPolicy())
	serverConn, clientConn := openPair(t, server, client, "svc")
	defer serverConn.Close()
	defer clientConn.Close()

	if id, ok := ServiceID(clientConn); !ok || id != "svc" {
		t.Fatalf("got service id %q, %v", id, ok)
	}

	want := []byte("hello through maek mux")
	go func() { _, _ = serverConn.Write(want) }()
	got := make([]byte, len(want))
	if _, err := io.ReadFull(clientConn, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStreamHalfClose(t *testing.T) {
	server, client := sessionPair(t, DefaultFlowPolicy())
	serverConn, clientConn := openPair(t, server, client, "svc")
	defer serverConn.Close()
	defer clientConn.Close()

	serverHalf := serverConn.(interface{ CloseWrite() error })
	clientHalf := clientConn.(interface{ CloseWrite() error })

	if _, err := serverConn.Write([]byte("request")); err != nil {
		t.Fatal(err)
	}
	if err := serverHalf.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	request, err := io.ReadAll(clientConn)
	if err != nil {
		t.Fatal(err)
	}
	if string(request) != "request" {
		t.Fatalf("got request %q", request)
	}

	if _, err := clientConn.Write([]byte("response")); err != nil {
		t.Fatal(err)
	}
	if err := clientHalf.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	response, err := io.ReadAll(serverConn)
	if err != nil {
		t.Fatal(err)
	}
	if string(response) != "response" {
		t.Fatalf("got response %q", response)
	}
}

func TestFlowControlStallDoesNotBlockOtherStreams(t *testing.T) {
	policy := FlowPolicy{
		MaxFramePayload:   1024,
		InitialWindow:     32 << 10,
		WindowUpdateBatch: 8 << 10,
		AcceptBacklog:     8,
	}
	server, client := sessionPair(t, policy)
	blockedServer, blockedClient := openPair(t, server, client, "blocked")
	defer blockedServer.Close()
	defer blockedClient.Close()
	fastServer, fastClient := openPair(t, server, client, "fast")
	defer fastServer.Close()
	defer fastClient.Close()

	blockedDone := make(chan error, 1)
	go func() {
		_, err := blockedServer.Write(make([]byte, int(policy.InitialWindow)*2))
		blockedDone <- err
	}()

	deadline := time.Now().Add(2 * time.Second)
	blockedStream := blockedServer.(*Stream)
	for {
		blockedStream.sendMu.Lock()
		window := blockedStream.sendWindow
		blockedStream.sendMu.Unlock()
		if window == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("blocked stream still has %d bytes of send credit", window)
		}
		time.Sleep(time.Millisecond)
	}

	fastClient.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := fastServer.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 4)
	if _, err := io.ReadFull(fastClient, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "ping" {
		t.Fatalf("got %q", got)
	}

	select {
	case err := <-blockedDone:
		if err == nil {
			t.Fatal("blocked stream unexpectedly completed without receiver reads")
		}
	default:
	}
}

func TestConcurrentStreams(t *testing.T) {
	server, client := sessionPair(t, DefaultFlowPolicy())
	const streams = 32

	acceptErr := make(chan error, 1)
	go func() {
		for i := 0; i < streams; i++ {
			conn, err := client.Listener().Accept()
			if err != nil {
				acceptErr <- err
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(conn)
		}
		acceptErr <- nil
	}()

	for i := 0; i < streams; i++ {
		conn, err := server.OpenHTTP(fmt.Sprintf("svc-%d", i))
		if err != nil {
			t.Fatal(err)
		}
		payload := []byte(fmt.Sprintf("message-%d", i))
		if _, err := conn.Write(payload); err != nil {
			conn.Close()
			t.Fatal(err)
		}
		got := make([]byte, len(payload))
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(conn, got); err != nil {
			conn.Close()
			t.Fatal(err)
		}
		conn.Close()
		if !bytes.Equal(got, payload) {
			t.Fatalf("got %q, want %q", got, payload)
		}
	}
	if err := <-acceptErr; err != nil {
		t.Fatal(err)
	}
}
