package protocol

import (
	"bytes"
	"reflect"
	"testing"
)

func TestSupportedSubprotocolsV2Only(t *testing.T) {
	if got, want := SupportedSubprotocols(), []string{"maek.v2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got, ok := ParseSubprotocol("maek.v2"); !ok || got != Version2 {
		t.Fatalf("got %d, %v", got, ok)
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	want := RegisterServices{
		SoftwareVersion: "test",
		Services:        []ServiceSpec{{Alias: "demo"}},
	}
	var buf bytes.Buffer
	if err := EncodeEnvelope(&buf, MsgRegister, want); err != nil {
		t.Fatal(err)
	}
	env, err := DecodeEnvelope(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != MsgRegister {
		t.Fatalf("got type %q", env.Type)
	}
	got, err := DecodePayload[RegisterServices](env)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
