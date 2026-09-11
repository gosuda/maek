package protocol

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

type Version uint16

const Version1 Version = 1

func (v Version) Subprotocol() string {
	return fmt.Sprintf("maek.v%d", v)
}

func SupportedVersions() []Version {
	return []Version{Version1}
}

func SupportedSubprotocols() []string {
	versions := SupportedVersions()
	out := make([]string, 0, len(versions))
	for _, v := range versions {
		out = append(out, v.Subprotocol())
	}
	return out
}

func ParseSubprotocol(s string) (Version, bool) {
	var n uint16
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "maek.v%d", &n); err != nil || n == 0 {
		return 0, false
	}
	return Version(n), true
}

type ContentEncoding string

const (
	EncodingIdentity ContentEncoding = "identity"
	EncodingGzip     ContentEncoding = "gzip"
)

func SupportedEncodings() []ContentEncoding {
	// Identity remains the default until compression is explicitly enabled by
	// configuration. Gzip is part of the v1 wire contract and can be negotiated.
	return []ContentEncoding{EncodingIdentity, EncodingGzip}
}

type Capability string

const (
	CapabilityMultiService   Capability = "multi-service"
	CapabilityStreamEncoding Capability = "stream-encoding"
)

func SupportedCapabilities() []Capability {
	return []Capability{CapabilityMultiService, CapabilityStreamEncoding}
}

type SessionConfig struct {
	Version      Version
	Encoding     ContentEncoding
	Capabilities []Capability
}

func NegotiateEncoding(peer, local []ContentEncoding) (ContentEncoding, bool) {
	peerSet := make(map[ContentEncoding]struct{}, len(peer))
	for _, v := range peer {
		peerSet[v] = struct{}{}
	}
	for _, v := range local {
		if _, ok := peerSet[v]; ok {
			return v, true
		}
	}
	return "", false
}

func IntersectCapabilities(a, b []Capability) []Capability {
	set := make(map[Capability]struct{}, len(b))
	for _, v := range b {
		set[v] = struct{}{}
	}
	out := make([]Capability, 0)
	for _, v := range a {
		if _, ok := set[v]; ok {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

const (
	MsgHello      = "hello"
	MsgWelcome    = "welcome"
	MsgRegisterV1 = "register"
	MsgRegistered = "registered"
	MsgStartV1    = "start"
	MsgErrorV1    = "error"
)

type Hello struct {
	SoftwareVersion string            `json:"software_version,omitempty"`
	Encodings       []ContentEncoding `json:"encodings"`
	Capabilities    []Capability      `json:"capabilities,omitempty"`
}

type Welcome struct {
	SoftwareVersion string          `json:"software_version,omitempty"`
	Encoding        ContentEncoding `json:"encoding"`
	Capabilities    []Capability    `json:"capabilities,omitempty"`
}

type ServiceSpec struct {
	PreferredID string `json:"preferred_id,omitempty"`
	Alias       string `json:"alias,omitempty"`
	Description string `json:"description,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
}

type RegisterServices struct {
	Services []ServiceSpec `json:"services"`
}

type AssignedService struct {
	Index uint16 `json:"index"`
	ID    string `json:"id"`
	Alias string `json:"alias"`
}

type RegisteredServices struct {
	Services []AssignedService `json:"services"`
}

type ProtocolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func EncodeEnvelope(w io.Writer, typ string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(Envelope{Type: typ, Payload: raw})
}

func DecodeEnvelope(r io.Reader) (Envelope, error) {
	var env Envelope
	if err := json.NewDecoder(r).Decode(&env); err != nil {
		return Envelope{}, fmt.Errorf("decode envelope: %w", err)
	}
	if env.Type == "" {
		return Envelope{}, fmt.Errorf("decode envelope: missing type")
	}
	return env, nil
}

func DecodePayload[T any](env Envelope) (T, error) {
	var out T
	if len(env.Payload) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(env.Payload, &out); err != nil {
		return out, fmt.Errorf("decode %s payload: %w", env.Type, err)
	}
	return out, nil
}
