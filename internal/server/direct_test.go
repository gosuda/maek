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

	// 5. Test with Encoded Space in Service Name: /_maek/code-server%20login
	{
		spaceInfo := protocol.ServiceInfo{
			ID:          "cs456",
			Name:        "code-server login",
			Description: "My Code Server Environment",
			Thumbnail:   "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			ConnectedAt: time.Now(),
		}
		_, _ = srv.Registry().Register(spaceInfo, nil, nil)

		// Regular browser gets 302 redirect with cookie
		req := httptest.NewRequest("GET", "/_maek/code-server%20login", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("expected status 302 for encoded space, got %d", rec.Code)
		}

		// OpenGraph crawler (e.g. Slackbot) gets 200 OK with rich OpenGraph tags
		botReq := httptest.NewRequest("GET", "/_maek/code-server%20login", nil)
		botReq.Header.Set("User-Agent", "Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)")
		botRec := httptest.NewRecorder()
		handler.ServeHTTP(botRec, botReq)

		if botRec.Code != http.StatusOK {
			t.Errorf("expected 200 OK for OpenGraph bot, got %d", botRec.Code)
		}
		body := botRec.Body.String()
		if !strings.Contains(body, `property="og:title" content="code-server login"`) {
			t.Errorf("expected og:title in body, got: %s", body)
		}
		if !strings.Contains(body, `property="og:description" content="My Code Server Environment"`) {
			t.Errorf("expected og:description in body, got: %s", body)
		}
		if !strings.Contains(body, `property="og:image" content="http://example.com/_maek/thumb?id=cs456"`) {
			t.Errorf("expected og:image endpoint in body, got: %s", body)
		}

		// Thumbnail endpoint returns raw decoded image bytes
		thumbReq := httptest.NewRequest("GET", "/_maek/thumb?id=cs456", nil)
		thumbRec := httptest.NewRecorder()
		handler.ServeHTTP(thumbRec, thumbReq)

		if thumbRec.Code != http.StatusOK {
			t.Errorf("expected status 200 for thumb, got %d", thumbRec.Code)
		}
		if thumbRec.Header().Get("Content-Type") != "image/png" {
			t.Errorf("expected image/png, got %s", thumbRec.Header().Get("Content-Type"))
		}
		if thumbRec.Body.Len() == 0 {
			t.Errorf("expected non-empty thumb image bytes")
		}
	}
}
