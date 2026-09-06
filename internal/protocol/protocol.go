package protocol

import (
	"crypto/rand"
	"math/big"
	"strings"
	"time"
)

const (
	CookieService  = "maek_service"
	HeaderService  = "X-Maek-Service"
	HeaderMaekID   = "X-Maek-ID"
	ReservedPrefix = "/_maek"

	EndpointWS       = "/_maek/ws"
	EndpointSelect   = "/_maek/select"
	EndpointExit     = "/_maek"
	EndpointFloatJS  = "/_maek/float.js"
	EndpointServices = "/_maek/api/services"
	EndpointVersion  = "/_maek/version"

	IDLength    = 6
	MaxIDLength = 32
	charset     = "abcdefghijklmnopqrstuvwxyz0123456789"
)

// ServiceInfo contains metadata about an active registered agent service.
type ServiceInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Thumbnail   string    `json:"thumbnail,omitempty"`
	ConnectedAt time.Time `json:"connected_at"`
	RemoteAddr  string    `json:"remote_addr"`
}

// GenerateID produces a random 6-character lowercase alphanumeric string.
func GenerateID() (string, error) {
	var sb strings.Builder
	sb.Grow(IDLength)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := 0; i < IDLength; i++ {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", err
		}
		sb.WriteByte(charset[n.Int64()])
	}
	return sb.String(), nil
}

// SanitizePreferredID strips invalid characters and ensures length <= MaxIDLength.
func SanitizePreferredID(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	var sb strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
			if sb.Len() >= MaxIDLength {
				break
			}
		}
	}
	return sb.String()
}

// IsReservedName checks if a name or ID conflicts with internal /_maek/* endpoints.
func IsReservedName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ws", "float.js", "float", "api", "version", "select":
		return true
	default:
		return false
	}
}
