package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

var streamMagic = [4]byte{'M', 'A', 'E', 'K'}

const MaxServiceIDLength = 64

type StreamKind uint8

const (
	StreamHTTP StreamKind = 1
)

type StreamHeader struct {
	Version   Version
	Kind      StreamKind
	Encoding  ContentEncoding
	ServiceID string
}

func encodingCode(enc ContentEncoding) (byte, error) {
	switch enc {
	case EncodingIdentity:
		return 0, nil
	case EncodingGzip:
		return 1, nil
	default:
		return 0, fmt.Errorf("unsupported content encoding %q", enc)
	}
}

func encodingFromCode(code byte) (ContentEncoding, error) {
	switch code {
	case 0:
		return EncodingIdentity, nil
	case 1:
		return EncodingGzip, nil
	default:
		return "", fmt.Errorf("unsupported content encoding code %d", code)
	}
}

func WriteStreamHeader(w io.Writer, h StreamHeader) error {
	if h.Version == 0 {
		return fmt.Errorf("stream header: missing protocol version")
	}
	if h.Kind == 0 {
		return fmt.Errorf("stream header: missing stream kind")
	}
	if len(h.ServiceID) > MaxServiceIDLength {
		return fmt.Errorf("stream header: service id too long")
	}
	enc, err := encodingCode(h.Encoding)
	if err != nil {
		return err
	}

	buf := make([]byte, 10+len(h.ServiceID))
	copy(buf[:4], streamMagic[:])
	binary.BigEndian.PutUint16(buf[4:6], uint16(h.Version))
	buf[6] = byte(h.Kind)
	buf[7] = enc
	binary.BigEndian.PutUint16(buf[8:10], uint16(len(h.ServiceID)))
	copy(buf[10:], h.ServiceID)
	_, err = w.Write(buf)
	return err
}

func ReadStreamHeader(r io.Reader) (StreamHeader, error) {
	fixed := make([]byte, 10)
	if _, err := io.ReadFull(r, fixed); err != nil {
		return StreamHeader{}, err
	}
	if string(fixed[:4]) != string(streamMagic[:]) {
		return StreamHeader{}, fmt.Errorf("stream header: invalid magic")
	}
	version := Version(binary.BigEndian.Uint16(fixed[4:6]))
	kind := StreamKind(fixed[6])
	encoding, err := encodingFromCode(fixed[7])
	if err != nil {
		return StreamHeader{}, err
	}
	idLen := int(binary.BigEndian.Uint16(fixed[8:10]))
	if idLen > MaxServiceIDLength {
		return StreamHeader{}, fmt.Errorf("stream header: service id too long")
	}
	id := make([]byte, idLen)
	if _, err := io.ReadFull(r, id); err != nil {
		return StreamHeader{}, err
	}
	return StreamHeader{Version: version, Kind: kind, Encoding: encoding, ServiceID: string(id)}, nil
}
