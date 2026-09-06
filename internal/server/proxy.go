package server

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// InjectFloatScript inserts the float.js script tag before </body> or </html>.
func InjectFloatScript(htmlContent []byte, serviceID, serviceName string) []byte {
	scriptTag := fmt.Sprintf(`<script src="/_maek/float.js" data-id="%s" data-name="%s" defer></script>`, serviceID, serviceName)

	bodyStr := string(htmlContent)
	bodyLower := strings.ToLower(bodyStr)

	// Try inserting before </body>
	if idx := strings.LastIndex(bodyLower, "</body>"); idx != -1 {
		return []byte(bodyStr[:idx] + scriptTag + bodyStr[idx:])
	}

	// Try inserting before </html>
	if idx := strings.LastIndex(bodyLower, "</html>"); idx != -1 {
		return []byte(bodyStr[:idx] + scriptTag + bodyStr[idx:])
	}

	// Fallback: append to end
	return append(htmlContent, []byte(scriptTag)...)
}

// BuildResponseModifier creates a ModifyResponse function tailored to a specific service.
func BuildResponseModifier(serviceID, serviceName string) func(*http.Response) error {
	return func(resp *http.Response) error {
		// Only inspect HTML responses
		cType := strings.ToLower(resp.Header.Get("Content-Type"))
		if !strings.Contains(cType, "text/html") {
			return nil
		}

		// Handle possible gzip encoding
		encoding := strings.ToLower(resp.Header.Get("Content-Encoding"))
		var reader io.Reader = resp.Body
		isGzip := strings.Contains(encoding, "gzip")

		if isGzip {
			gzReader, err := gzip.NewReader(resp.Body)
			if err != nil {
				// If gzip fails to parse, return original body
				return nil
			}
			defer gzReader.Close()
			reader = gzReader
		}

		rawBody, err := io.ReadAll(reader)
		if err != nil {
			return nil
		}
		_ = resp.Body.Close()

		// Inject float script
		modified := InjectFloatScript(rawBody, serviceID, serviceName)

		// Clean up headers
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Security-Policy")
		resp.Header.Del("Content-Security-Policy-Report-Only")

		resp.Body = io.NopCloser(bytes.NewReader(modified))
		resp.ContentLength = int64(len(modified))
		resp.Header.Set("Content-Length", strconv.Itoa(len(modified)))

		return nil
	}
}
