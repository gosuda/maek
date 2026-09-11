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
	input := "<html><body><h1>Hello</h1></body></html>"
	got := string(InjectFloatScript([]byte(input), "abc123", "webapp"))
	want := `<script src="/_maek/float.js" data-id="abc123" data-alias="webapp" data-name="webapp" defer></script>`
	if !strings.Contains(got, want) {
		t.Fatalf("got %q, expected %q", got, want)
	}
}

func TestBuildResponseModifierGzip(t *testing.T) {
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
	if err := BuildResponseModifier("k8x2p9", "my-app")(resp); err != nil {
		t.Fatal(err)
	}
	if resp.Header.Get("Content-Encoding") != "" || resp.Header.Get("Content-Security-Policy") != "" {
		t.Fatal("expected modified response headers to be stripped")
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `data-alias="my-app"`) {
		t.Fatalf("alias metadata not injected: %s", body)
	}
}

func TestBuildResponseModifierNonHTML(t *testing.T) {
	const payload = `{"status":"ok"}`
	resp := &http.Response{Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(payload))}
	if err := BuildResponseModifier("id", "alias")(resp); err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != payload {
		t.Fatalf("got %q", body)
	}
}

func TestBuildResponseModifierRootNoCache(t *testing.T) {
	newResp := func(path string) *http.Response {
		return &http.Response{
			Header: http.Header{"Content-Type": []string{"text/html"}, "Cache-Control": []string{"public, max-age=3600"}},
			Body: io.NopCloser(strings.NewReader("<html><body>ok</body></html>")),
			Request: &http.Request{URL: &url.URL{Path: path}},
		}
	}
	root := newResp("/")
	if err := BuildResponseModifier("id", "alias")(root); err != nil {
		t.Fatal(err)
	}
	if got := root.Header.Get("Cache-Control"); got != "no-cache, no-store, must-revalidate" {
		t.Fatalf("root Cache-Control=%q", got)
	}
	sub := newResp("/assets/app.js")
	if err := BuildResponseModifier("id", "alias")(sub); err != nil {
		t.Fatal(err)
	}
	if got := sub.Header.Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("subpath Cache-Control=%q", got)
	}
}
