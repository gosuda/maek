package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	metaCSPRegex    = regexp.MustCompile(`(?i)<meta[^>]+http-equiv=["']?Content-Security-Policy["']?[^>]*>`)
	domainAttrRegex = regexp.MustCompile(`(?i);\s*Domain=[^;]+`)
	secureAttrRegex = regexp.MustCompile(`(?i);\s*Secure`)
)

type HTTPStreamOpener interface {
	OpenHTTP(serviceID string) (net.Conn, error)
}

func BuildTunnelProxy(opener HTTPStreamOpener, serviceID, serviceAlias string) *httputil.ReverseProxy {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return opener.OpenHTTP(serviceID)
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "maek"
			if req.Header.Get("Accept-Encoding") != "" {
				req.Header.Set("Accept-Encoding", "gzip")
			}
		},
		Transport:      transport,
		ModifyResponse: BuildResponseModifier(serviceID, serviceAlias),
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("502 Bad Gateway - maek could not reach agent: " + err.Error()))
		},
	}
}

func InjectFloatScript(htmlContent []byte, serviceID, serviceAlias string) []byte {
	cleaned := metaCSPRegex.ReplaceAll(htmlContent, nil)
	// data-alias is the v1 contract. data-name is temporarily mirrored for the
	// existing self-contained float widget, which is otherwise protocol-agnostic.
	scriptTag := fmt.Sprintf(`<script src="/_maek/float.js" data-id="%s" data-alias="%s" data-name="%s" defer></script>`, serviceID, serviceAlias, serviceAlias)
	bodyStr := string(cleaned)
	bodyLower := strings.ToLower(bodyStr)
	if idx := strings.LastIndex(bodyLower, "</body>"); idx != -1 {
		return []byte(bodyStr[:idx] + scriptTag + bodyStr[idx:])
	}
	if idx := strings.LastIndex(bodyLower, "</html>"); idx != -1 {
		return []byte(bodyStr[:idx] + scriptTag + bodyStr[idx:])
	}
	return append(cleaned, []byte(scriptTag)...)
}

func BuildResponseModifier(serviceID, serviceAlias string) func(*http.Response) error {
	return func(resp *http.Response) error {
		if resp.Request != nil && resp.Request.URL.Path == "/" {
			resp.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
			resp.Header.Set("Pragma", "no-cache")
			resp.Header.Set("Expires", "0")
		}
		if cookies := resp.Header["Set-Cookie"]; len(cookies) > 0 {
			newCookies := make([]string, len(cookies))
			for i, c := range cookies {
				cleaned := domainAttrRegex.ReplaceAllString(c, "")
				cleaned = secureAttrRegex.ReplaceAllString(cleaned, "")
				newCookies[i] = cleaned
			}
			resp.Header["Set-Cookie"] = newCookies
		}
		cType := strings.ToLower(resp.Header.Get("Content-Type"))
		if !strings.Contains(cType, "text/html") {
			return nil
		}
		encoding := strings.ToLower(resp.Header.Get("Content-Encoding"))
		var reader io.Reader = resp.Body
		isGzip := strings.Contains(encoding, "gzip")
		if isGzip {
			gzReader, err := gzip.NewReader(resp.Body)
			if err != nil {
				return nil
			}
			defer gzReader.Close()
			reader = gzReader
		} else if encoding != "" && encoding != "identity" {
			return nil
		}
		rawBody, err := io.ReadAll(reader)
		if err != nil {
			return nil
		}
		_ = resp.Body.Close()
		modified := InjectFloatScript(rawBody, serviceID, serviceAlias)
		if isGzip {
			resp.Header.Del("Content-Encoding")
		}
		resp.Header.Del("Content-Security-Policy")
		resp.Header.Del("Content-Security-Policy-Report-Only")
		resp.Header.Del("Etag")
		resp.Body = io.NopCloser(bytes.NewReader(modified))
		resp.ContentLength = int64(len(modified))
		resp.Header.Set("Content-Length", strconv.Itoa(len(modified)))
		return nil
	}
}
