package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gosuda/maek/internal/protocol"
)

func TestHandleLLMsTxt(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	handler := srv.Handler()

	// 1. Served with the requesting origin templated in
	req := httptest.NewRequest("GET", protocol.EndpointLLMsTxt, nil)
	req.Host = "tunnel.example.com:8080"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("expected text/plain content-type, got %q", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "http://tunnel.example.com:8080") {
		t.Errorf("expected templated server URL in llms.txt")
	}
	if strings.Contains(body, "__SERVER_URL__") {
		t.Errorf("expected __SERVER_URL__ to be replaced, but it was found in body")
	}
	if !strings.Contains(body, "maek agent --server http://tunnel.example.com:8080") {
		t.Errorf("expected ready-to-run agent command in llms.txt")
	}

	// 2. Behind an HTTPS reverse proxy (X-Forwarded-Proto)
	reqTLS := httptest.NewRequest("GET", protocol.EndpointLLMsTxt, nil)
	reqTLS.Host = "tunnel.example.com"
	reqTLS.Header.Set("X-Forwarded-Proto", "https")
	recTLS := httptest.NewRecorder()
	handler.ServeHTTP(recTLS, reqTLS)

	if recTLS.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recTLS.Code)
	}
	if !strings.Contains(recTLS.Body.String(), "https://tunnel.example.com") {
		t.Errorf("expected https origin templated behind proxy")
	}
}

func TestHandleStyleCSS(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", protocol.EndpointStyleCSS, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/css") {
		t.Errorf("expected text/css content-type, got %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "--accent") {
		t.Errorf("expected stylesheet content")
	}
}

func TestHandleAppJS(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", protocol.EndpointAppJS, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Errorf("expected javascript content-type, got %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "renderServices") {
		t.Errorf("expected dashboard script content")
	}
}
