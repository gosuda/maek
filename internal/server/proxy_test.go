package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestInjectFloatScript(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		serviceID   string
		serviceName string
		expected    string
	}{
		{
			name:        "Standard body close",
			input:       "<html><head></head><body><h1>Hello</h1></body></html>",
			serviceID:   "abc123",
			serviceName: "webapp",
			expected:    `<html><head></head><body><h1>Hello</h1><script src="/_maek/float.js" data-id="abc123" data-name="webapp" defer></script></body></html>`,
		},
		{
			name:        "No body, has html close",
			input:       "<html><div>Content</div></html>",
			serviceID:   "xyz789",
			serviceName: "api-ui",
			expected:    `<html><div>Content</div><script src="/_maek/float.js" data-id="xyz789" data-name="api-ui" defer></script></html>`,
		},
		{
			name:        "Fragment without body or html tag",
			input:       "<div>Just a fragment</div>",
			serviceID:   "xyz789",
			serviceName: "api-ui",
			expected:    `<div>Just a fragment</div><script src="/_maek/float.js" data-id="xyz789" data-name="api-ui" defer></script>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := InjectFloatScript([]byte(tt.input), tt.serviceID, tt.serviceName)
			if string(output) != tt.expected {
				t.Errorf("got %q, want %q", string(output), tt.expected)
			}
		})
	}
}

func TestBuildResponseModifier_Gzip(t *testing.T) {
	originalHTML := "<html><body><h1>Gzipped Page</h1></body></html>"
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	_, _ = gzWriter.Write([]byte(originalHTML))
	_ = gzWriter.Close()

	resp := &http.Response{
		Header: http.Header{
			"Content-Type":            []string{"text/html; charset=utf-8"},
			"Content-Encoding":        []string{"gzip"},
			"Content-Security-Policy": []string{"default-src 'self'"},
		},
		Body: io.NopCloser(&buf),
	}

	modifier := BuildResponseModifier("k8x2p9", "my-app")
	err := modifier(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Content-Encoding and CSP should be stripped
	if resp.Header.Get("Content-Encoding") != "" {
		t.Errorf("expected Content-Encoding to be removed, got %q", resp.Header.Get("Content-Encoding"))
	}
	if resp.Header.Get("Content-Security-Policy") != "" {
		t.Errorf("expected Content-Security-Policy to be removed, got %q", resp.Header.Get("Content-Security-Policy"))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading modified body: %v", err)
	}
	result := string(bodyBytes)

	if !strings.Contains(result, `<script src="/_maek/float.js" data-id="k8x2p9" data-name="my-app" defer></script></body></html>`) {
		t.Errorf("expected injected script, got: %s", result)
	}
}

func TestBuildResponseModifier_NonHTML(t *testing.T) {
	jsonPayload := `{"status":"ok"}`
	resp := &http.Response{
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(jsonPayload)),
	}

	modifier := BuildResponseModifier("k8x2p9", "my-app")
	err := modifier(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading body: %v", err)
	}
	if string(bodyBytes) != jsonPayload {
		t.Errorf("expected body to remain unchanged, got %q", string(bodyBytes))
	}
}

func TestBuildResponseModifier_RootNoCache(t *testing.T) {
	newResp := func(path string) *http.Response {
		return &http.Response{
			Header: http.Header{
				"Content-Type":  []string{"text/html; charset=utf-8"},
				"Cache-Control": []string{"public, max-age=3600"},
			},
			Body: io.NopCloser(strings.NewReader("<html><body>ok</body></html>")),
			Request: &http.Request{
				URL: &url.URL{Path: path},
			},
		}
	}

	// 1. Root path: upstream cache headers must be overridden.
	rootResp := newResp("/")
	if err := BuildResponseModifier("abc123", "app")(rootResp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cc := rootResp.Header.Get("Cache-Control"); cc != "no-cache, no-store, must-revalidate" {
		t.Errorf("root: expected forced no-cache, got %q", cc)
	}
	if rootResp.Header.Get("Expires") != "0" {
		t.Errorf("root: expected Expires: 0, got %q", rootResp.Header.Get("Expires"))
	}

	// 2. Non-root path: upstream cache headers must be preserved.
	subResp := newResp("/assets/app.js")
	if err := BuildResponseModifier("abc123", "app")(subResp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cc := subResp.Header.Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Errorf("subpath: expected upstream Cache-Control preserved, got %q", cc)
	}
}
