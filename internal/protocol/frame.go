package protocol

import (
	"encoding/binary"
	"fmt"
)

const (
	FrameHeaderSize       = 5
	MaxServiceIDLength    = 64
	MaxDataFramePayload   = 1 << 20
	MaxStreamWindowCredit = 64 << 20
)

type FrameType uint8

const (
	FrameOpen FrameType = iota + 1
	FrameData
	FrameFIN
	FrameRST
	FrameWindowUpdate
)

type Frame struct {
	Type     FrameType
	StreamID uint32
	Payload  []byte
}

func EncodeFrame(typ FrameType, streamID uint32, payload []byte) ([]byte, error) {
	if streamID == 0 {
		return nil, fmt.Errorf("frame: stream id must be non-zero")
	}
	if err := validateFramePayload(typ, payload); err != nil {
		return nil, err
	}
	buf := make([]byte, FrameHeaderSize+len(payload))
	buf[0] = byte(typ)
	binary.BigEndian.PutUint32(buf[1:5], streamID)
	copy(buf[FrameHeaderSize:], payload)
	return buf, nil
}

func DecodeFrame(buf []byte) (Frame, error) {
	if len(buf) < FrameHeaderSize {
		return Frame{}, fmt.Errorf("frame: short header")
	}
	frame := Frame{
		Type:     FrameType(buf[0]),
		StreamID: binary.BigEndian.Uint32(buf[1:5]),
		Payload:  buf[FrameHeaderSize:],
	}
	if frame.StreamID == 0 {
		return Frame{}, fmt.Errorf("frame: stream id must be non-zero")
	}
	if err := validateFramePayload(frame.Type, frame.Payload); err != nil {
		return Frame{}, err
	}
	return frame, nil
}

func EncodeWindowUpdate(delta uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, delta)
	return buf
}

func DecodeWindowUpdate(payload []byte) (uint32, error) {
	if len(payload) != 4 {
		return 0, fmt.Errorf("window update: want 4 bytes, got %d", len(payload))
	}
	delta := binary.BigEndian.Uint32(payload)
	if delta == 0 {
		return 0, fmt.Errorf("window update: zero credit")
	}
	return delta, nil
}

func validateFramePayload(typ FrameType, payload []byte) error {
	switch typ {
	case FrameOpen:
		if len(payload) == 0 {
			return fmt.Errorf("open frame: missing service id")
		}
		if len(payload) > MaxServiceIDLength {
			return fmt.Errorf("open frame: service id too long")
		}
	case FrameData:
		if len(payload) == 0 {
			return fmt.Errorf("data frame: empty payload")
		}
		if len(payload) > MaxDataFramePayload {
			return fmt.Errorf("data frame: payload too large")
		}
	case FrameFIN, FrameRST:
		if len(payload) != 0 {
			return fmt.Errorf("frame type %d: unexpected payload", typ)
		}
	case FrameWindowUpdate:
		if _, err := DecodeWindowUpdate(payload); err != nil {
			return err
		}
	default:
		return fmt.Errorf("frame: unknown type %d", typ)
	}
	return nil
}
