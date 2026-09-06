package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gosuda/maek/internal/protocol"
	"github.com/gosuda/maek/internal/version"
)

func TestHandleVersion(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	handler := srv.Handler()

	// 1. JSON response (default)
	req := httptest.NewRequest("GET", protocol.EndpointVersion, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Errorf("expected application/json, got %q", rec.Header().Get("Content-Type"))
	}

	var info version.Info
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if info.Version != version.Version {
		t.Errorf("expected version %q, got %q", version.Version, info.Version)
	}

	// 2. Text response (Accept: text/plain)
	reqText := httptest.NewRequest("GET", protocol.EndpointVersion, nil)
	reqText.Header.Set("Accept", "text/plain")
	recText := httptest.NewRecorder()
	handler.ServeHTTP(recText, reqText)

	if recText.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recText.Code)
	}
	if !strings.Contains(recText.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("expected text/plain, got %q", recText.Header().Get("Content-Type"))
	}
	bodyStr := strings.TrimSpace(recText.Body.String())
	if !strings.HasPrefix(bodyStr, "maek version") {
		t.Errorf("expected body to start with 'maek version', got %q", bodyStr)
	}
}
