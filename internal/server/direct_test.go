package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gosuda/maek/internal/protocol"
	"github.com/gosuda/maek/internal/service"
)

func registerDirectTestService(t *testing.T, srv *Server, info service.Info) *Registration {
	t.Helper()
	reservation, err := srv.Registry().ReserveID(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	info.ID = reservation.ID()
	_, registration, err := srv.Registry().Activate(reservation, info, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Close)
	return registration
}

func TestDirectServiceURLUsesAlias(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	registerDirectTestService(t, srv, service.Info{ID: "svc123", Alias: "my-service", ConnectedAt: time.Now()})
	handler := srv.Handler()

	for _, path := range []string{"/_maek/my-service", "/_maek/svc123"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound {
			t.Fatalf("%s: expected 302, got %d", path, rec.Code)
		}
		if got := rec.Header().Get("Location"); got != "/" {
			t.Fatalf("%s: location %q", path, got)
		}
	}
}

func TestDirectAliasWithSubpath(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	registerDirectTestService(t, srv, service.Info{ID: "svc123", Alias: "my-service", ConnectedAt: time.Now()})
	req := httptest.NewRequest("GET", "/_maek/my-service/api/docs?page=2", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/api/docs?page=2" {
		t.Fatalf("got status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestDirectDuplicateAliasUsesOldest(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	now := time.Now()
	registerDirectTestService(t, srv, service.Info{ID: "owner-a", Alias: "shared", ConnectedAt: now})
	registerDirectTestService(t, srv, service.Info{ID: "owner-b", Alias: "shared", ConnectedAt: now.Add(time.Second)})

	req := httptest.NewRequest("GET", "/_maek/shared", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	cookies := rec.Result().Cookies()
	for _, cookie := range cookies {
		if cookie.Name == protocol.CookieService {
			if cookie.Value != "owner-a" {
				t.Fatalf("alias resolved to %q, want owner-a", cookie.Value)
			}
			return
		}
	}
	t.Fatal("routing cookie not set")
}

func TestDirectAliasOpenGraphAndThumbnail(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	registerDirectTestService(t, srv, service.Info{
		ID:          "cs456",
		Alias:       "code-server login",
		Description: "My Code Server Environment",
		Thumbnail:   "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
		ConnectedAt: time.Now(),
	})

	botReq := httptest.NewRequest("GET", "/_maek/code-server%20login", nil)
	botReq.Header.Set("User-Agent", "Slackbot-LinkExpanding")
	botRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(botRec, botReq)
	if botRec.Code != http.StatusOK || !strings.Contains(botRec.Body.String(), `property="og:title" content="code-server login"`) {
		t.Fatalf("unexpected OpenGraph response: %d %s", botRec.Code, botRec.Body.String())
	}

	thumbReq := httptest.NewRequest("GET", "/_maek/thumb?id=cs456", nil)
	thumbRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(thumbRec, thumbReq)
	if thumbRec.Code != http.StatusOK || thumbRec.Header().Get("Content-Type") != "image/png" || thumbRec.Body.Len() == 0 {
		t.Fatalf("unexpected thumbnail response")
	}
}
