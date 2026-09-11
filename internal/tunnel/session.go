package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/gosuda/maek/internal/protocol"
)

var (
	ErrSessionClosed = errors.New("maek tunnel session closed")
	ErrStreamReset   = errors.New("maek tunnel stream reset")
	ErrAcceptBacklog = errors.New("maek tunnel accept backlog full")
)

type sessionRole uint8

const (
	roleServer sessionRole = iota + 1
	roleClient
)

type Session struct {
	ws     *websocket.Conn
	role   sessionRole
	policy FlowPolicy

	ctx    context.Context
	cancel context.CancelFunc

	writeGate chan struct{}
	acceptCh  chan net.Conn
	done      chan struct{}

	streamsMu sync.RWMutex
	streams   map[uint32]*Stream

	idMu   sync.Mutex
	nextID uint32

	errMu sync.RWMutex
	err   error

	closeOnce sync.Once
}

func NewServer(conn *websocket.Conn) (*Session, error) {
	return newSession(conn, roleServer, DefaultFlowPolicy())
}

func NewClient(conn *websocket.Conn) (*Session, error) {
	return newSession(conn, roleClient, DefaultFlowPolicy())
}

func newSession(conn *websocket.Conn, role sessionRole, policy FlowPolicy) (*Session, error) {
	if conn == nil {
		return nil, fmt.Errorf("websocket connection is required")
	}
	if err := policy.validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		ws:        conn,
		role:      role,
		policy:    policy,
		ctx:       ctx,
		cancel:    cancel,
		writeGate: make(chan struct{}, 1),
		acceptCh:  make(chan net.Conn, policy.AcceptBacklog),
		done:      make(chan struct{}),
		streams:   make(map[uint32]*Stream),
	}
	s.writeGate <- struct{}{}
	if role == roleServer {
		s.nextID = 1
	} else {
		s.nextID = 2
	}
	go s.readLoop()
	return s, nil
}

func (s *Session) OpenHTTP(serviceID string) (net.Conn, error) {
	if serviceID == "" || len(serviceID) > protocol.MaxServiceIDLength || !utf8.ValidString(serviceID) {
		return nil, fmt.Errorf("invalid service id")
	}
	if err := s.sessionErr(); err != nil {
		return nil, err
	}

	id, err := s.allocateStreamID()
	if err != nil {
		return nil, err
	}
	stream := newStream(s, id, serviceID, s.policy)
	if err := s.addStream(stream); err != nil {
		return nil, err
	}
	if err := s.writeFrame(time.Time{}, protocol.FrameOpen, id, []byte(serviceID)); err != nil {
		s.removeStream(id, stream)
		stream.abort(err)
		return nil, err
	}
	return stream, nil
}

func (s *Session) Listener() net.Listener {
	return &Listener{session: s}
}

func (s *Session) Done() <-chan struct{} { return s.done }

func (s *Session) Err() error { return s.sessionErr() }

func (s *Session) Close() error {
	s.shutdown(ErrSessionClosed)
	return nil
}

func (s *Session) allocateStreamID() (uint32, error) {
	s.idMu.Lock()
	defer s.idMu.Unlock()
	id := s.nextID
	if id == 0 || id > ^uint32(0)-2 {
		return 0, fmt.Errorf("stream id space exhausted")
	}
	s.nextID += 2
	return id, nil
}

func (s *Session) validRemoteStreamID(id uint32) bool {
	if s.role == roleServer {
		return id%2 == 0
	}
	return id%2 == 1
}

func (s *Session) addStream(stream *Stream) error {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	if _, exists := s.streams[stream.id]; exists {
		return fmt.Errorf("stream %d already exists", stream.id)
	}
	s.streams[stream.id] = stream
	return nil
}

func (s *Session) getStream(id uint32) *Stream {
	s.streamsMu.RLock()
	stream := s.streams[id]
	s.streamsMu.RUnlock()
	return stream
}

func (s *Session) removeStream(id uint32, stream *Stream) {
	s.streamsMu.Lock()
	if current := s.streams[id]; current == stream {
		delete(s.streams, id)
	}
	s.streamsMu.Unlock()
}

func (s *Session) readLoop() {
	for {
		typ, raw, err := s.ws.Read(s.ctx)
		if err != nil {
			s.shutdown(err)
			return
		}
		if typ != websocket.MessageBinary {
			s.shutdown(fmt.Errorf("unexpected websocket message type %d in data plane", typ))
			return
		}
		frame, err := protocol.DecodeFrame(raw)
		if err != nil {
			s.shutdown(err)
			return
		}
		if err := s.handleFrame(frame); err != nil {
			s.shutdown(err)
			return
		}
	}
}

func (s *Session) handleFrame(frame protocol.Frame) error {
	switch frame.Type {
	case protocol.FrameOpen:
		return s.handleOpen(frame)
	case protocol.FrameData:
		stream := s.getStream(frame.StreamID)
		if stream == nil {
			return nil
		}
		if err := stream.pushData(frame.Payload); err != nil {
			s.removeStream(frame.StreamID, stream)
			stream.abort(err)
			_ = s.writeFrame(time.Time{}, protocol.FrameRST, frame.StreamID, nil)
		}
	case protocol.FrameFIN:
		if stream := s.getStream(frame.StreamID); stream != nil {
			stream.markRemoteFIN()
		}
	case protocol.FrameRST:
		if stream := s.getStream(frame.StreamID); stream != nil {
			s.removeStream(frame.StreamID, stream)
			stream.abort(ErrStreamReset)
		}
	case protocol.FrameWindowUpdate:
		stream := s.getStream(frame.StreamID)
		if stream == nil {
			return nil
		}
		delta, err := protocol.DecodeWindowUpdate(frame.Payload)
		if err != nil {
			return err
		}
		if err := stream.addSendWindow(delta); err != nil {
			s.removeStream(frame.StreamID, stream)
			stream.abort(err)
			_ = s.writeFrame(time.Time{}, protocol.FrameRST, frame.StreamID, nil)
		}
	}
	return nil
}

func (s *Session) handleOpen(frame protocol.Frame) error {
	if !s.validRemoteStreamID(frame.StreamID) {
		return fmt.Errorf("invalid remote stream id %d", frame.StreamID)
	}
	serviceID := string(frame.Payload)
	if !utf8.ValidString(serviceID) {
		return fmt.Errorf("invalid utf-8 service id")
	}
	stream := newStream(s, frame.StreamID, serviceID, s.policy)
	if err := s.addStream(stream); err != nil {
		return err
	}
	select {
	case s.acceptCh <- stream:
		return nil
	default:
		s.removeStream(frame.StreamID, stream)
		stream.abort(ErrAcceptBacklog)
		_ = s.writeFrame(time.Time{}, protocol.FrameRST, frame.StreamID, nil)
		return nil
	}
}

func (s *Session) writeFrame(deadline time.Time, typ protocol.FrameType, streamID uint32, payload []byte) error {
	raw, err := protocol.EncodeFrame(typ, streamID, payload)
	if err != nil {
		return err
	}

	var timer *time.Timer
	var timeout <-chan time.Time
	if !deadline.IsZero() {
		d := time.Until(deadline)
		if d <= 0 {
			return os.ErrDeadlineExceeded
		}
		timer = time.NewTimer(d)
		timeout = timer.C
		defer timer.Stop()
	}

	select {
	case <-s.done:
		return s.sessionErr()
	case <-timeout:
		return os.ErrDeadlineExceeded
	case <-s.writeGate:
	}
	defer func() { s.writeGate <- struct{}{} }()

	ctx := s.ctx
	var cancel context.CancelFunc
	if !deadline.IsZero() {
		ctx, cancel = context.WithDeadline(s.ctx, deadline)
		defer cancel()
	}
	if err := s.ws.Write(ctx, websocket.MessageBinary, raw); err != nil {
		s.shutdown(err)
		return err
	}
	return nil
}

func (s *Session) shutdown(err error) {
	if err == nil {
		err = ErrSessionClosed
	}
	s.closeOnce.Do(func() {
		s.errMu.Lock()
		s.err = err
		s.errMu.Unlock()
		s.cancel()
		s.ws.CloseNow()

		s.streamsMu.Lock()
		streams := make([]*Stream, 0, len(s.streams))
		for _, stream := range s.streams {
			streams = append(streams, stream)
		}
		s.streams = make(map[uint32]*Stream)
		s.streamsMu.Unlock()
		for _, stream := range streams {
			stream.abort(err)
		}
		close(s.done)
	})
}

func (s *Session) sessionErr() error {
	select {
	case <-s.done:
		s.errMu.RLock()
		err := s.err
		s.errMu.RUnlock()
		if err != nil {
			return err
		}
		return ErrSessionClosed
	default:
		return nil
	}
}

type Listener struct {
	session *Session
}

func (l *Listener) Accept() (net.Conn, error) {
	select {
	case <-l.session.done:
		return nil, l.session.sessionErr()
	default:
	}
	select {
	case <-l.session.done:
		return nil, l.session.sessionErr()
	case conn := <-l.session.acceptCh:
		return conn, nil
	}
}

func (l *Listener) Close() error   { return l.session.Close() }
func (l *Listener) Addr() net.Addr { return tunnelAddr{} }

type tunnelAddr struct{}

func (tunnelAddr) Network() string { return "maek" }
func (tunnelAddr) String() string  { return "maek-tunnel" }
