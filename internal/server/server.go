package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/gosuda/maek/internal/protocol"
	"github.com/gosuda/maek/internal/version"
	"github.com/hashicorp/yamux"
)

type Config struct {
	Addr string
}

type Server struct {
	addr     string
	registry *Registry
	httpSrv  *http.Server
}

func NewServer(cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	s := &Server{
		addr:     cfg.Addr,
		registry: NewRegistry(),
	}
	return s
}

// Registry returns the active service registry.
func (s *Server) Registry() *Registry {
	return s.registry
}

// Handler returns the master HTTP handler for the server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Reserved endpoints
	mux.HandleFunc(protocol.EndpointWS, s.handleAgentWebSocket)
	mux.HandleFunc(protocol.EndpointFloatJS, s.handleFloatJS)
	mux.HandleFunc(protocol.EndpointServices, s.handleServicesAPI)
	mux.HandleFunc(protocol.EndpointVersion, s.handleVersion)
	mux.HandleFunc(protocol.EndpointSelect, s.handleSelectService)
	mux.HandleFunc(protocol.EndpointExit, s.handleExitService)

	// Fallback catch-all handler for Web UI and reverse-proxying
	mux.HandleFunc("/", s.handleProxyOrCatalog)

	return mux
}

// Start runs the HTTP server listening on the configured address.
func (s *Server) Start() error {
	s.httpSrv = &http.Server{
		Addr:    s.addr,
		Handler: s.Handler(),
	}
	log.Printf("[maek-server] Listening on http://%s", s.addr)
	return s.httpSrv.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(ctx)
	}
	return nil
}

// handleAgentWebSocket handles incoming agent WebSocket connections.
func (s *Server) handleAgentWebSocket(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = "app"
	}
	desc := strings.TrimSpace(r.URL.Query().Get("desc"))
	thumb := strings.TrimSpace(r.URL.Query().Get("thumb"))
	preferredID := strings.TrimSpace(r.URL.Query().Get("id"))
	if preferredID == "" {
		preferredID = name
	}

	id, err := s.registry.AllocateID(preferredID)
	if err != nil {
		http.Error(w, "Failed to allocate service ID", http.StatusInternalServerError)
		return
	}

	// Send assigned ID in the response headers
	w.Header().Set(protocol.HeaderMaekID, id)

	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("[maek-server] WebSocket handshake failed: %v", err)
		return
	}

	netConn := websocket.NetConn(context.Background(), wsConn, websocket.MessageBinary)

	yamuxCfg := yamux.DefaultConfig()
	yamuxCfg.LogOutput = io.Discard
	session, err := yamux.Server(netConn, yamuxCfg)
	if err != nil {
		log.Printf("[maek-server] Failed to create yamux server: %v", err)
		_ = wsConn.Close(websocket.StatusInternalError, "yamux init failed")
		return
	}

	info := protocol.ServiceInfo{
		ID:          id,
		Name:        name,
		Description: desc,
		Thumbnail:   thumb,
		ConnectedAt: time.Now(),
		RemoteAddr:  r.RemoteAddr,
	}

	_, err = s.registry.Register(info, session, BuildResponseModifier(id, name))
	if err != nil {
		log.Printf("[maek-server] Failed to register service: %v", err)
		_ = session.Close()
		return
	}

	log.Printf("[maek-server] Registered agent '%s' (ID: %s) from %s", name, id, r.RemoteAddr)

	// Block until connection is closed
	<-session.CloseChan()
	s.registry.Unregister(id)
	log.Printf("[maek-server] Agent '%s' (ID: %s) disconnected", name, id)
}

// handleFloatJS serves the floating widget JavaScript.
func (s *Server) handleFloatJS(w http.ResponseWriter, r *http.Request) {
	data, err := GetStaticFile("float.js")
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

// handleServicesAPI returns a JSON list of active services.
func (s *Server) handleServicesAPI(w http.ResponseWriter, r *http.Request) {
	services := s.registry.List()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(services)
}

// handleVersion returns the build version and commit info in JSON or plain text.
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	info := version.Get()
	if r.Header.Get("Accept") == "text/plain" || r.URL.Query().Get("format") == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, version.String())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(info)
}

// handleSelectService sets the routing cookie and redirects to /.
func (s *Server) handleSelectService(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if _, ok := s.registry.Get(id); !ok {
		http.Error(w, "Service not found or disconnected", http.StatusNotFound)
		return
	}

	// Session cookie (no MaxAge, no Expires - cleared on browser close)
	http.SetCookie(w, &http.Cookie{
		Name:     protocol.CookieService,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

// handleExitService removes the routing cookie and redirects to /.
func (s *Server) handleExitService(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     protocol.CookieService,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleProxyOrCatalog decides whether to show the catalog or proxy to an agent.
func (s *Server) handleProxyOrCatalog(w http.ResponseWriter, r *http.Request) {
	// 1. Check custom header (priority for CLI / API)
	serviceKey := strings.TrimSpace(r.Header.Get(protocol.HeaderService))

	// 2. Check session cookie
	if serviceKey == "" {
		if cookie, err := r.Cookie(protocol.CookieService); err == nil {
			serviceKey = strings.TrimSpace(cookie.Value)
		}
	}

	// If no service requested:
	if serviceKey == "" {
		if r.URL.Path == "/" {
			s.serveCatalogUI(w, r)
			return
		}
		// For unrouted subpaths, redirect back to catalog
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Look up service in registry
	session, ok := s.registry.Get(serviceKey)
	if !ok {
		// If service is no longer connected, clear cookie and notify
		http.SetCookie(w, &http.Cookie{
			Name:     protocol.CookieService,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Service '%s' is not found or has disconnected. <a href=\"/\">Return to Catalog</a>", serviceKey)
		return
	}

	// Proxy to target agent
	session.ReverseProxy.ServeHTTP(w, r)
}

// serveCatalogUI serves the default index.html dashboard.
func (s *Server) serveCatalogUI(w http.ResponseWriter, r *http.Request) {
	data, err := GetStaticFile("index.html")
	if err != nil {
		http.Error(w, "Web UI not available", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
