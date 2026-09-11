package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gosuda/maek/internal/protocol"
)

func TestHandleInstallScripts(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	for _, endpoint := range []string{protocol.EndpointInstallSh, protocol.EndpointInstallPs1} {
		req := httptest.NewRequest("GET", endpoint, nil)
		req.Host = "tunnel.example.com:8080"
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", endpoint, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "http://tunnel.example.com:8080") || strings.Contains(rec.Body.String(), "__SERVER_URL__") {
			t.Fatalf("%s: server URL was not templated", endpoint)
		}
	}
}

func TestReservedInstallerEndpoints(t *testing.T) {
	for _, handle := range []string{"install.sh", "install.ps1", "download"} {
		if !protocol.IsReservedHandle(handle) {
			t.Errorf("expected %q to be reserved", handle)
		}
	}
}
