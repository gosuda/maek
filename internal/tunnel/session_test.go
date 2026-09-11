package tunnel

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/gosuda/maek/internal/protocol"
)

func TestGzipConnRoundTrip(t *testing.T) {
	leftRaw, rightRaw := net.Pipe()
	left, err := wrapEncoding(leftRaw, protocol.EncodingGzip)
	if err != nil {
		t.Fatal(err)
	}
	right, err := wrapEncoding(rightRaw, protocol.EncodingGzip)
	if err != nil {
		t.Fatal(err)
	}
	defer left.Close()
	defer right.Close()

	payload := []byte("hello through gzip")
	errCh := make(chan error, 1)
	go func() {
		_, err := left.Write(payload)
		errCh <- err
	}()

	_ = right.SetReadDeadline(time.Now().Add(time.Second))
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(right, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("got %q, want %q", got, payload)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestServiceID(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	conn := &serviceConn{Conn: left, serviceID: "svc"}
	id, ok := ServiceID(conn)
	if !ok || id != "svc" {
		t.Fatalf("got %q, %v", id, ok)
	}
}
