package protocol

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type Version uint16

const Version2 Version = 2

func (v Version) Subprotocol() string {
	return fmt.Sprintf("maek.v%d", v)
}

func SupportedVersions() []Version {
	return []Version{Version2}
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

const (
	MsgRegister   = "register"
	MsgRegistered = "registered"
	MsgError      = "error"
)

type ServiceSpec struct {
	Alias       string `json:"alias,omitempty"`
	Description string `json:"description,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
}

type RegisterServices struct {
	SoftwareVersion string        `json:"software_version,omitempty"`
	Services        []ServiceSpec `json:"services"`
}

type AssignedService struct {
	Index uint16 `json:"index"`
	ID    string `json:"id"`
	Alias string `json:"alias"`
}

type RegisteredServices struct {
	SoftwareVersion string            `json:"software_version,omitempty"`
	Services        []AssignedService `json:"services"`
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
