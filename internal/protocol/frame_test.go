package protocol

import (
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	payload := []byte("hello")
	raw, err := EncodeFrame(FrameData, 7, payload)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != FrameData || got.StreamID != 7 || !bytes.Equal(got.Payload, payload) {
		t.Fatalf("unexpected frame: %+v", got)
	}
}

func TestWindowUpdateRoundTrip(t *testing.T) {
	payload := EncodeWindowUpdate(256 << 10)
	got, err := DecodeWindowUpdate(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got != 256<<10 {
		t.Fatalf("got %d", got)
	}
}

func TestFrameRejectsOversizedData(t *testing.T) {
	_, err := EncodeFrame(FrameData, 1, make([]byte, MaxDataFramePayload+1))
	if err == nil {
		t.Fatal("expected oversized data error")
	}
}
