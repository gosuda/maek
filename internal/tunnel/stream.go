package tunnel

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/gosuda/maek/internal/protocol"
)

type Stream struct {
	session   *Session
	id        uint32
	serviceID string
	policy    FlowPolicy

	recvMu          sync.Mutex
	recvBuf         bytes.Buffer
	recvErr         error
	remoteFIN       bool
	recvSinceUpdate uint32
	recvNotify      chan struct{}

	sendMu     sync.Mutex
	sendWindow uint32
	sendErr    error
	localFIN   bool
	sendNotify chan struct{}

	deadlineMu    sync.RWMutex
	readDeadline  time.Time
	writeDeadline time.Time

	closeOnce sync.Once
}

func newStream(session *Session, id uint32, serviceID string, policy FlowPolicy) *Stream {
	return &Stream{
		session:    session,
		id:         id,
		serviceID:  serviceID,
		policy:     policy,
		sendWindow: policy.InitialWindow,
		recvNotify: make(chan struct{}, 1),
		sendNotify: make(chan struct{}, 1),
	}
}

func ServiceID(conn net.Conn) (string, bool) {
	type serviceIdentifier interface{ ServiceID() string }
	if c, ok := conn.(serviceIdentifier); ok {
		return c.ServiceID(), true
	}
	return "", false
}

func (s *Stream) ServiceID() string { return s.serviceID }

func (s *Stream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		s.recvMu.Lock()
		if s.recvBuf.Len() > 0 {
			n, _ := s.recvBuf.Read(p)
			s.recvSinceUpdate += uint32(n)
			var update uint32
			if s.recvSinceUpdate >= s.policy.WindowUpdateBatch {
				update = s.recvSinceUpdate
				s.recvSinceUpdate = 0
			}
			s.recvMu.Unlock()
			if update > 0 {
				if err := s.session.writeFrame(time.Time{}, protocol.FrameWindowUpdate, s.id, protocol.EncodeWindowUpdate(update)); err != nil {
					return n, nil
				}
			}
			return n, nil
		}
		if s.recvErr != nil {
			err := s.recvErr
			s.recvMu.Unlock()
			return 0, err
		}
		if s.remoteFIN {
			s.recvMu.Unlock()
			return 0, io.EOF
		}
		s.recvMu.Unlock()

		if err := s.waitForRead(); err != nil {
			return 0, err
		}
	}
}

func (s *Stream) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	written := 0
	for written < len(p) {
		n, err := s.reserveSend(len(p) - written)
		if err != nil {
			return written, err
		}
		deadline := s.getWriteDeadline()
		if err := s.session.writeFrame(deadline, protocol.FrameData, s.id, p[written:written+n]); err != nil {
			return written, err
		}
		written += n
		if written < len(p) {
			runtime.Gosched()
		}
	}
	return written, nil
}

func (s *Stream) CloseWrite() error {
	s.sendMu.Lock()
	if s.sendErr != nil {
		err := s.sendErr
		s.sendMu.Unlock()
		return err
	}
	if s.localFIN {
		s.sendMu.Unlock()
		return nil
	}
	s.localFIN = true
	s.sendMu.Unlock()
	return s.session.writeFrame(s.getWriteDeadline(), protocol.FrameFIN, s.id, nil)
}

func (s *Stream) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		s.abort(net.ErrClosed)
		s.session.removeStream(s.id, s)
		select {
		case <-s.session.Done():
			return
		default:
		}
		closeErr = s.session.writeFrame(s.getWriteDeadline(), protocol.FrameRST, s.id, nil)
	})
	return closeErr
}

func (s *Stream) LocalAddr() net.Addr  { return tunnelAddr{} }
func (s *Stream) RemoteAddr() net.Addr { return tunnelAddr{} }

func (s *Stream) SetDeadline(t time.Time) error {
	s.deadlineMu.Lock()
	s.readDeadline = t
	s.writeDeadline = t
	s.deadlineMu.Unlock()
	s.notifyRead()
	s.notifySend()
	return nil
}

func (s *Stream) SetReadDeadline(t time.Time) error {
	s.deadlineMu.Lock()
	s.readDeadline = t
	s.deadlineMu.Unlock()
	s.notifyRead()
	return nil
}

func (s *Stream) SetWriteDeadline(t time.Time) error {
	s.deadlineMu.Lock()
	s.writeDeadline = t
	s.deadlineMu.Unlock()
	s.notifySend()
	return nil
}

func (s *Stream) pushData(payload []byte) error {
	s.recvMu.Lock()
	defer s.recvMu.Unlock()
	if s.recvErr != nil {
		return s.recvErr
	}
	if s.remoteFIN {
		return fmt.Errorf("stream %d: data after FIN", s.id)
	}
	if uint64(s.recvBuf.Len())+uint64(len(payload)) > uint64(s.policy.InitialWindow) {
		return fmt.Errorf("stream %d: receive window exceeded", s.id)
	}
	_, _ = s.recvBuf.Write(payload)
	s.notify(s.recvNotify)
	return nil
}

func (s *Stream) markRemoteFIN() {
	s.recvMu.Lock()
	s.remoteFIN = true
	s.recvMu.Unlock()
	s.notifyRead()
}

func (s *Stream) addSendWindow(delta uint32) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if s.sendErr != nil {
		return s.sendErr
	}
	if delta > protocol.MaxStreamWindowCredit-s.sendWindow {
		return fmt.Errorf("stream %d: send window overflow", s.id)
	}
	s.sendWindow += delta
	s.notify(s.sendNotify)
	return nil
}

func (s *Stream) reserveSend(remaining int) (int, error) {
	for {
		s.sendMu.Lock()
		if s.sendErr != nil {
			err := s.sendErr
			s.sendMu.Unlock()
			return 0, err
		}
		if s.localFIN {
			s.sendMu.Unlock()
			return 0, io.ErrClosedPipe
		}
		if s.sendWindow > 0 {
			n := remaining
			if n > s.policy.MaxFramePayload {
				n = s.policy.MaxFramePayload
			}
			if uint32(n) > s.sendWindow {
				n = int(s.sendWindow)
			}
			s.sendWindow -= uint32(n)
			s.sendMu.Unlock()
			return n, nil
		}
		s.sendMu.Unlock()

		if err := s.waitForWrite(); err != nil {
			return 0, err
		}
	}
}

func (s *Stream) abort(err error) {
	if err == nil {
		err = ErrStreamReset
	}
	s.recvMu.Lock()
	if s.recvErr == nil {
		s.recvErr = err
		s.recvBuf.Reset()
	}
	s.recvMu.Unlock()

	s.sendMu.Lock()
	if s.sendErr == nil {
		s.sendErr = err
	}
	s.sendMu.Unlock()

	s.notifyRead()
	s.notifySend()
}

func (s *Stream) waitForRead() error {
	return s.waitForSignal(s.recvNotify, s.getReadDeadline())
}

func (s *Stream) waitForWrite() error {
	return s.waitForSignal(s.sendNotify, s.getWriteDeadline())
}

func (s *Stream) waitForSignal(signal <-chan struct{}, deadline time.Time) error {
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
	case <-signal:
		return nil
	case <-s.session.Done():
		if err := s.session.Err(); err != nil {
			return err
		}
		return ErrSessionClosed
	case <-timeout:
		return os.ErrDeadlineExceeded
	}
}

func (s *Stream) getReadDeadline() time.Time {
	s.deadlineMu.RLock()
	deadline := s.readDeadline
	s.deadlineMu.RUnlock()
	return deadline
}

func (s *Stream) getWriteDeadline() time.Time {
	s.deadlineMu.RLock()
	deadline := s.writeDeadline
	s.deadlineMu.RUnlock()
	return deadline
}

func (s *Stream) notifyRead() { s.notify(s.recvNotify) }
func (s *Stream) notifySend() { s.notify(s.sendNotify) }
func (s *Stream) notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
