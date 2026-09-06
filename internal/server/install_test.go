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
	handler := srv.Handler()

	// 1. Shell install script
	reqSh := httptest.NewRequest("GET", protocol.EndpointInstallSh, nil)
	reqSh.Host = "tunnel.example.com:8080"
	recSh := httptest.NewRecorder()
	handler.ServeHTTP(recSh, reqSh)

	if recSh.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recSh.Code)
	}
	if !strings.Contains(recSh.Header().Get("Content-Type"), "text/x-shellscript") {
		t.Errorf("expected shellscript content-type, got %q", recSh.Header().Get("Content-Type"))
	}
	bodySh := recSh.Body.String()
	if !strings.Contains(bodySh, "http://tunnel.example.com:8080") {
		t.Errorf("expected templated server URL in script, got body snippet:\n%s", bodySh[:200])
	}
	if strings.Contains(bodySh, "__SERVER_URL__") {
		t.Errorf("expected __SERVER_URL__ to be replaced, but it was found in body")
	}

	// 2. PowerShell install script
	reqPs := httptest.NewRequest("GET", protocol.EndpointInstallPs1, nil)
	reqPs.Host = "tunnel.example.com:8080"
	recPs := httptest.NewRecorder()
	handler.ServeHTTP(recPs, reqPs)

	if recPs.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recPs.Code)
	}
	bodyPs := recPs.Body.String()
	if !strings.Contains(bodyPs, "http://tunnel.example.com:8080") {
		t.Errorf("expected templated server URL in PowerShell script")
	}
	if strings.Contains(bodyPs, "__SERVER_URL__") {
		t.Errorf("expected __SERVER_URL__ to be replaced, but it was found in body")
	}
}

func TestHandleDownload(t *testing.T) {
	srv := NewServer(Config{Addr: ":0"})
	handler := srv.Handler()

	// 1. Darwin arm64 (embedded)
	reqDarwin := httptest.NewRequest("GET", protocol.EndpointDownload+"?os=Darwin&arch=arm64", nil)
	recDarwin := httptest.NewRecorder()
	handler.ServeHTTP(recDarwin, reqDarwin)

	if recDarwin.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recDarwin.Code)
	}
	if !strings.Contains(recDarwin.Header().Get("Content-Type"), "application/gzip") {
		t.Errorf("expected gzip content-type, got %q", recDarwin.Header().Get("Content-Type"))
	}
	if !strings.Contains(recDarwin.Header().Get("Content-Disposition"), "maek_Darwin_arm64.tar.gz") {
		t.Errorf("expected Content-Disposition filename maek_Darwin_arm64.tar.gz, got %q", recDarwin.Header().Get("Content-Disposition"))
	}

	// 2. Windows x86_64 (embedded zip)
	reqWin := httptest.NewRequest("GET", protocol.EndpointDownload+"?os=Windows&arch=x86_64", nil)
	recWin := httptest.NewRecorder()
	handler.ServeHTTP(recWin, reqWin)

	if recWin.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recWin.Code)
	}
	if !strings.Contains(recWin.Header().Get("Content-Type"), "application/zip") {
		t.Errorf("expected zip content-type, got %q", recWin.Header().Get("Content-Type"))
	}
	if !strings.Contains(recWin.Header().Get("Content-Disposition"), "maek_Windows_x86_64.zip") {
		t.Errorf("expected Content-Disposition filename maek_Windows_x86_64.zip, got %q", recWin.Header().Get("Content-Disposition"))
	}

	// 3. Linux x86_64 (embedded tar.gz)
	reqLinux := httptest.NewRequest("GET", protocol.EndpointDownload+"?os=Linux&arch=x86_64", nil)
	recLinux := httptest.NewRecorder()
	handler.ServeHTTP(recLinux, reqLinux)

	if recLinux.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recLinux.Code)
	}
	if !strings.Contains(recLinux.Header().Get("Content-Type"), "application/gzip") {
		t.Errorf("expected gzip content-type, got %q", recLinux.Header().Get("Content-Type"))
	}
}

func TestReservedInstallerEndpoints(t *testing.T) {
	endpoints := []string{"install.sh", "install.ps1", "download"}
	for _, ep := range endpoints {
		if !protocol.IsReservedName(ep) {
			t.Errorf("expected %q to be reserved, but was not", ep)
		}
	}
}
