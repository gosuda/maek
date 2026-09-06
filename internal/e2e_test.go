package internal_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gosuda/maek/internal/agent"
	"github.com/gosuda/maek/internal/protocol"
	"github.com/gosuda/maek/internal/server"
)

func TestE2E_TunnelLifecycle(t *testing.T) {
	// 1. Mock Local Target Server
	targetMux := http.NewServeMux()
	targetMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Write([]byte("<html><head><title>My Local App</title></head><body><h1>Welcome</h1></body></html>"))
	})
	targetMux.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"hello from local"}`))
	})
	targetServer := httptest.NewServer(targetMux)
	defer targetServer.Close()

	// 2. Start maek server on random free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	serverAddr := listener.Addr().String()

	srv := server.NewServer(server.Config{
		Addr: serverAddr,
	})

	httpSrv := &http.Server{
		Handler: srv.Handler(),
	}
	go func() {
		_ = httpSrv.Serve(listener)
	}()
	defer httpSrv.Close()

	// 3. Before Agent connects: check catalog page
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't follow redirects
		},
	}

	res, err := client.Get("http://" + serverAddr + "/")
	if err != nil {
		t.Fatalf("failed to GET /: %v", err)
	}
	catalogHTML, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(catalogHTML), "Tunnels") {
		t.Fatalf("expected Catalog Web UI, got: %s", string(catalogHTML))
	}

	// 4. Start maek agent
	agCtx, agCancel := context.WithCancel(context.Background())
	defer agCancel()

	ag, err := agent.NewAgent(agent.Config{
		ServerURL: "ws://" + serverAddr,
		Name:      "test-service",
		Target:    targetServer.URL,
	})
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	go func() {
		_ = ag.Start(agCtx)
	}()

	// Wait for agent to register
	var serviceID string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		services := srv.Registry().List()
		if len(services) > 0 {
			serviceID = services[0].ID
			break
		}
	}
	if serviceID == "" {
		t.Fatalf("agent did not register within deadline")
	}

	// 5. Test /_maek/api/services endpoint
	res, err = client.Get("http://" + serverAddr + protocol.EndpointServices)
	if err != nil {
		t.Fatalf("failed to fetch services: %v", err)
	}
	var svcList []protocol.ServiceInfo
	json.NewDecoder(res.Body).Decode(&svcList)
	res.Body.Close()

	if len(svcList) != 1 || svcList[0].Name != "test-service" || svcList[0].ID != serviceID {
		t.Fatalf("unexpected services list: %+v", svcList)
	}

	// 6. Test select service (/_maek/select?id=xxx)
	selectURL := fmt.Sprintf("http://%s%s?id=%s", serverAddr, protocol.EndpointSelect, serviceID)
	res, err = client.Get(selectURL)
	if err != nil {
		t.Fatalf("select service failed: %v", err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusFound {
		t.Fatalf("expected 302 Found, got %d", res.StatusCode)
	}

	var sessionCookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == protocol.CookieService {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value != serviceID {
		t.Fatalf("expected session cookie with value %s, got: %+v", serviceID, sessionCookie)
	}

	// 7. Request / with session cookie -> verify proxying and float.js injection
	req, _ := http.NewRequest("GET", "http://"+serverAddr+"/", nil)
	req.AddCookie(sessionCookie)
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	proxiedBody, _ := io.ReadAll(res.Body)
	res.Body.Close()

	bodyStr := string(proxiedBody)
	if !strings.Contains(bodyStr, "<h1>Welcome</h1>") {
		t.Fatalf("expected target response body, got: %s", bodyStr)
	}

	expectedScript := fmt.Sprintf(`<script src="/_maek/float.js" data-id="%s" data-name="test-service" defer></script></body></html>`, serviceID)
	if !strings.Contains(bodyStr, expectedScript) {
		t.Fatalf("expected injected float script %q, got: %s", expectedScript, bodyStr)
	}

	if res.Header.Get("Content-Security-Policy") != "" {
		t.Errorf("expected Content-Security-Policy to be stripped, got %s", res.Header.Get("Content-Security-Policy"))
	}

	// 8. Request /api/hello with session cookie -> JSON should NOT have script injected
	req, _ = http.NewRequest("GET", "http://"+serverAddr+"/api/hello", nil)
	req.AddCookie(sessionCookie)
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("json request failed: %v", err)
	}
	jsonBody, _ := io.ReadAll(res.Body)
	res.Body.Close()

	if string(jsonBody) != `{"message":"hello from local"}` {
		t.Fatalf("unexpected JSON body: %s", string(jsonBody))
	}

	// 9. Test Header-based routing without cookie (X-Maek-Service: test-service)
	req, _ = http.NewRequest("GET", "http://"+serverAddr+"/api/hello", nil)
	req.Header.Set(protocol.HeaderService, "test-service")
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("header routed request failed: %v", err)
	}
	headerJson, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(headerJson) != `{"message":"hello from local"}` {
		t.Fatalf("unexpected response from header routing: %s", string(headerJson))
	}

	// 10. Test float.js serving (GET /_maek/float.js)
	res, err = client.Get("http://" + serverAddr + protocol.EndpointFloatJS)
	if err != nil {
		t.Fatalf("failed to fetch float.js: %v", err)
	}
	floatJSBytes, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(floatJSBytes), "maek-float-root") {
		t.Fatalf("expected valid float.js, got status %d, body: %s", res.StatusCode, string(floatJSBytes))
	}

	// 11. Test Exit (GET /_maek) -> clears cookie and redirects to /
	req, _ = http.NewRequest("GET", "http://"+serverAddr+protocol.EndpointExit, nil)
	req.AddCookie(sessionCookie)
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("failed to exit: %v", err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusFound {
		t.Fatalf("expected 302 Found on exit, got %d", res.StatusCode)
	}

	var expiredCookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == protocol.CookieService {
			expiredCookie = c
			break
		}
	}
	if expiredCookie == nil || expiredCookie.MaxAge >= 0 {
		t.Fatalf("expected cookie to be expired (Max-Age < 0), got: %+v", expiredCookie)
	}

	// 12. Stop Agent -> verify disconnection
	agCancel()
	ag.Stop()

	// Wait for unregister
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if len(srv.Registry().List()) == 0 {
			break
		}
	}
	if len(srv.Registry().List()) != 0 {
		t.Fatalf("expected 0 services after agent stopped, got %d", len(srv.Registry().List()))
	}
}
