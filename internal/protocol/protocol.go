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

	IDLength = 6
	charset  = "abcdefghijklmnopqrstuvwxyz0123456789"
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
