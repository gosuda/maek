package internal_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gosuda/maek/internal/agent"
	"github.com/gosuda/maek/internal/protocol"
	"github.com/gosuda/maek/internal/server"
)

func newTarget(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(body))
	}))
}

func waitForServices(t *testing.T, srv *server.Server, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.Registry().List()) == n {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("got %d services, want %d", len(srv.Registry().List()), n)
}

func routedGET(t *testing.T, baseURL, serviceKey string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(protocol.HeaderService, serviceKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("route %q returned %d: %s", serviceKey, resp.StatusCode, body)
	}
	return string(body)
}

func TestE2E_MultiServiceSessionAndAliasRouting(t *testing.T) {
	targetA := newTarget(t, "target-a")
	defer targetA.Close()
	targetB := newTarget(t, "target-b")
	defer targetB.Close()

	srv := server.NewServer(server.Config{Addr: ":0"})
	master := httptest.NewServer(srv.Handler())
	defer master.Close()

	ag, err := agent.NewAgent(agent.Config{
		ServerURL: master.URL,
		Services: []agent.ServiceConfig{
			{Alias: "shared", Target: targetA.URL},
			{Alias: "shared", Target: targetB.URL},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ag.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	})

	waitForServices(t, srv, 2)
	services := srv.Registry().List()
	if services[0].ID != "shared" || services[1].ID != "shared-2" {
		t.Fatalf("unexpected alias-derived IDs: %+v", services)
	}
	if got := routedGET(t, master.URL, "shared"); got != "target-a" {
		t.Fatalf("shared routed to %q", got)
	}
	if got := routedGET(t, master.URL, "shared-2"); got != "target-b" {
		t.Fatalf("shared-2 routed to %q", got)
	}

	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && len(srv.Registry().List()) != 0 {
		time.Sleep(25 * time.Millisecond)
	}
	if got := len(srv.Registry().List()); got != 0 {
		t.Fatalf("got %d services after disconnect", got)
	}
}
