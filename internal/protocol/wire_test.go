package protocol

import (
	"bytes"
	"reflect"
	"testing"
)

func TestNegotiateEncodingUsesLocalPreference(t *testing.T) {
	got, ok := NegotiateEncoding([]ContentEncoding{EncodingGzip, EncodingIdentity}, []ContentEncoding{EncodingIdentity, EncodingGzip})
	if !ok || got != EncodingIdentity {
		t.Fatalf("got %q, %v", got, ok)
	}
}

func TestIntersectCapabilities(t *testing.T) {
	got := IntersectCapabilities(
		[]Capability{CapabilityStreamEncoding, CapabilityMultiService},
		[]Capability{CapabilityMultiService},
	)
	want := []Capability{CapabilityMultiService}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestStreamHeaderRoundTrip(t *testing.T) {
	want := StreamHeader{Version: Version1, Kind: StreamHTTP, Encoding: EncodingGzip, ServiceID: "demo"}
	var buf bytes.Buffer
	if err := WriteStreamHeader(&buf, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadStreamHeader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
