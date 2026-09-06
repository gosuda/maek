package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gosuda/maek/internal/protocol"
)

func TestDirectServiceURL(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})

	// Register a mock service directly in registry
	info := protocol.ServiceInfo{
		ID:          "svc123",
		Name:        "my-service",
		ConnectedAt: time.Now(),
	}
	_, err := srv.Registry().Register(info, nil, nil)
	if err != nil {
		t.Fatalf("failed to register mock service: %v", err)
	}

	handler := srv.Handler()

	// 1. Test by Service Name: /_maek/my-service
	{
		req := httptest.NewRequest("GET", "/_maek/my-service", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("expected status 302, got %d", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Errorf("expected redirect to '/', got %q", loc)
		}

		cookies := rec.Result().Cookies()
		foundCookie := false
		for _, c := range cookies {
			if c.Name == protocol.CookieService && c.Value == "svc123" {
				foundCookie = true
				break
			}
		}
		if !foundCookie {
			t.Errorf("expected maek_service cookie with value 'svc123'")
		}
	}

	// 2. Test by Service ID: /_maek/svc123
	{
		req := httptest.NewRequest("GET", "/_maek/svc123", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("expected status 302, got %d", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Errorf("expected redirect to '/', got %q", loc)
		}
	}

	// 3. Test with Subpath: /_maek/my-service/api/docs?page=2
	{
		req := httptest.NewRequest("GET", "/_maek/my-service/api/docs?page=2", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("expected status 302, got %d", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/api/docs?page=2" {
			t.Errorf("expected redirect to '/api/docs?page=2', got %q", loc)
		}
	}

	// 4. Test non-existent service: /_maek/ghost-service
	{
		req := httptest.NewRequest("GET", "/_maek/ghost-service", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "not found") {
			t.Errorf("expected 'not found' message, got %q", rec.Body.String())
		}
	}
}
