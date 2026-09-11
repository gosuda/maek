package protocol

import (
	"crypto/rand"
	"math/big"
	"strings"
)

const (
	CookieService  = "maek_service"
	HeaderService  = "X-Maek-Service"
	ReservedPrefix = "/_maek"

	EndpointWS         = "/_maek/ws"
	EndpointSelect     = "/_maek/select"
	EndpointExit       = "/_maek"
	EndpointFloatJS    = "/_maek/float.js"
	EndpointServices   = "/_maek/api/services"
	EndpointVersion    = "/_maek/version"
	EndpointThumb      = "/_maek/thumb"
	EndpointInstallSh  = "/_maek/install.sh"
	EndpointInstallPs1 = "/_maek/install.ps1"
	EndpointLLMsTxt    = "/_maek/llms.txt"
	EndpointStyleCSS   = "/_maek/style.css"
	EndpointAppJS      = "/_maek/app.js"
	EndpointDownload   = "/_maek/download"

	IDLength    = 6
	MaxIDLength = 32
	charset     = "abcdefghijklmnopqrstuvwxyz0123456789"
)

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

func IsReservedHandle(handle string) bool {
	switch strings.ToLower(strings.TrimSpace(handle)) {
	case "ws", "float.js", "float", "api", "version", "select", "thumb", "install.sh", "install.ps1", "download", "llms.txt", "style.css", "app.js":
		return true
	default:
		return false
	}
}
