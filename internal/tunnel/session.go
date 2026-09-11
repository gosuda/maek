package tunnel

import (
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/gosuda/maek/internal/protocol"
	"github.com/hashicorp/yamux"
)

type Session struct {
	mux    *yamux.Session
	config protocol.SessionConfig
}

func NewClient(conn net.Conn, config protocol.SessionConfig) (*Session, error) {
	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	mux, err := yamux.Client(conn, cfg)
	if err != nil {
		return nil, err
	}
	return &Session{mux: mux, config: config}, nil
}

func NewServer(conn net.Conn, config protocol.SessionConfig) (*Session, error) {
	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	mux, err := yamux.Server(conn, cfg)
	if err != nil {
		return nil, err
	}
	return &Session{mux: mux, config: config}, nil
}

func (s *Session) Config() protocol.SessionConfig { return s.config }

func (s *Session) OpenHTTP(serviceID string) (net.Conn, error) {
	raw, err := s.mux.Open()
	if err != nil {
		return nil, err
	}
	header := protocol.StreamHeader{
		Version:   s.config.Version,
		Kind:      protocol.StreamHTTP,
		Encoding:  s.config.Encoding,
		ServiceID: serviceID,
	}
	if err := protocol.WriteStreamHeader(raw, header); err != nil {
		_ = raw.Close()
		return nil, err
	}
	conn, err := wrapEncoding(raw, s.config.Encoding)
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	return &serviceConn{Conn: conn, serviceID: serviceID}, nil
}

func (s *Session) Listener() net.Listener {
	return &Listener{session: s}
}

func (s *Session) Done() <-chan struct{} { return s.mux.CloseChan() }
func (s *Session) Close() error           { return s.mux.Close() }

type Listener struct {
	session *Session
}

func (l *Listener) Accept() (net.Conn, error) {
	for {
		raw, err := l.session.mux.Accept()
		if err != nil {
			return nil, err
		}
		header, err := protocol.ReadStreamHeader(raw)
		if err != nil {
			_ = raw.Close()
			return nil, fmt.Errorf("read stream header: %w", err)
		}
		if header.Version != l.session.config.Version || header.Kind != protocol.StreamHTTP {
			_ = raw.Close()
			continue
		}
		if header.Encoding != l.session.config.Encoding {
			_ = raw.Close()
			return nil, fmt.Errorf("stream encoding %q does not match negotiated encoding %q", header.Encoding, l.session.config.Encoding)
		}
		conn, err := wrapEncoding(raw, header.Encoding)
		if err != nil {
			_ = raw.Close()
			return nil, err
		}
		return &serviceConn{Conn: conn, serviceID: header.ServiceID}, nil
	}
}

func (l *Listener) Close() error   { return l.session.Close() }
func (l *Listener) Addr() net.Addr { return l.session.mux.Addr() }

type serviceConn struct {
	net.Conn
	serviceID string
}

func ServiceID(conn net.Conn) (string, bool) {
	type serviceIdentifier interface{ ServiceID() string }
	if c, ok := conn.(serviceIdentifier); ok {
		return c.ServiceID(), true
	}
	return "", false
}

func (c *serviceConn) ServiceID() string { return c.serviceID }

func wrapEncoding(conn net.Conn, encoding protocol.ContentEncoding) (net.Conn, error) {
	switch encoding {
	case protocol.EncodingIdentity:
		return conn, nil
	case protocol.EncodingGzip:
		return &gzipConn{Conn: conn}, nil
	default:
		return nil, fmt.Errorf("unsupported content encoding %q", encoding)
	}
}

type gzipConn struct {
	net.Conn
	readOnce sync.Once
	reader   *gzip.Reader
	readErr  error
	writeMu  sync.Mutex
	writer   *gzip.Writer
	closeMu  sync.Mutex
	closed   bool
}

func (c *gzipConn) Read(p []byte) (int, error) {
	c.readOnce.Do(func() {
		c.reader, c.readErr = gzip.NewReader(c.Conn)
	})
	if c.readErr != nil {
		return 0, c.readErr
	}
	return c.reader.Read(p)
}

func (c *gzipConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.writer == nil {
		c.writer = gzip.NewWriter(c.Conn)
	}
	n, err := c.writer.Write(p)
	if err != nil {
		return n, err
	}
	if err := c.writer.Flush(); err != nil {
		return n, err
	}
	return n, nil
}

func (c *gzipConn) Close() error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return nil
	}
	c.closed = true
	c.closeMu.Unlock()

	c.writeMu.Lock()
	if c.writer != nil {
		_ = c.writer.Close()
	}
	c.writeMu.Unlock()
	if c.reader != nil {
		_ = c.reader.Close()
	}
	return c.Conn.Close()
}
