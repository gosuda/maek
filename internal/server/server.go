package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
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
	mux.HandleFunc(protocol.EndpointThumb, s.handleThumb)
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

// handleThumb serves decoded thumbnail images from Base64 Data URIs or redirects to external URLs.
func (s *Server) handleThumb(w http.ResponseWriter, r *http.Request) {
	idOrName := strings.TrimSpace(r.URL.Query().Get("id"))
	if idOrName == "" {
		http.NotFound(w, r)
		return
	}
	session, ok := s.registry.Get(idOrName)
	if !ok || session.Info.Thumbnail == "" {
		http.NotFound(w, r)
		return
	}

	thumb := session.Info.Thumbnail
	if strings.HasPrefix(thumb, "data:") {
		// data:<mime>;base64,<encoded-data>
		parts := strings.SplitN(thumb, ",", 2)
		if len(parts) == 2 {
			mime := "image/png"
			meta := strings.TrimPrefix(parts[0], "data:")
			metaParts := strings.Split(meta, ";")
			if len(metaParts) > 0 && metaParts[0] != "" {
				mime = metaParts[0]
			}

			data, err := base64.StdEncoding.DecodeString(parts[1])
			if err == nil {
				w.Header().Set("Content-Type", mime)
				w.Header().Set("Cache-Control", "public, max-age=86400")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
				return
			}
		}
	} else if strings.HasPrefix(thumb, "http://") || strings.HasPrefix(thumb, "https://") {
		http.Redirect(w, r, thumb, http.StatusFound)
		return
	}

	http.NotFound(w, r)
}

// isBot detects if an incoming request is from a social media crawler or bot.
func isBot(ua string) bool {
	if ua == "" {
		return false
	}
	ua = strings.ToLower(ua)
	botSignatures := []string{
		"bot", "crawler", "spider", "scraper",
		"facebookexternalhit", "twitterbot", "slackbot",
		"discordbot", "telegrambot", "kakaotalk-scrap",
		"whatsapp", "linkedinbot", "meta-externalagent",
		"applebot", "bingbot", "googlebot", "yandex",
		"embedly", "quora link preview", "showyoubot",
		"outbrain", "pinterest", "vkshare",
	}
	for _, sig := range botSignatures {
		if strings.Contains(ua, sig) {
			return true
		}
	}
	return false
}

// serveOpenGraphHTML serves a dedicated HTML page containing OpenGraph metadata for scrapers.
func (s *Server) serveOpenGraphHTML(w http.ResponseWriter, r *http.Request, session *ServiceSession) {
	host := r.Host
	if host == "" {
		host = s.addr
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	pageURL := fmt.Sprintf("%s://%s%s", scheme, host, r.URL.RequestURI())
	imageURL := ""
	if session.Info.Thumbnail != "" {
		if strings.HasPrefix(session.Info.Thumbnail, "http://") || strings.HasPrefix(session.Info.Thumbnail, "https://") {
			imageURL = session.Info.Thumbnail
		} else {
			imageURL = fmt.Sprintf("%s://%s%s?id=%s", scheme, host, protocol.EndpointThumb, session.Info.ID)
		}
	}

	title := html.EscapeString(session.Info.Name)
	desc := html.EscapeString(session.Info.Description)
	if desc == "" {
		desc = fmt.Sprintf("Tunnel to %s hosted via maek", session.Info.Name)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>%s</title>
  <meta property="og:type" content="website">
  <meta property="og:site_name" content="maek">
  <meta property="og:title" content="%s">
  <meta property="og:description" content="%s">
  <meta property="og:url" content="%s">
`, title, title, desc, html.EscapeString(pageURL))

	if imageURL != "" {
		fmt.Fprintf(w, `  <meta property="og:image" content="%s">
  <meta name="twitter:image" content="%s">
`, html.EscapeString(imageURL), html.EscapeString(imageURL))
	}

	fmt.Fprintf(w, `  <meta name="description" content="%s">
  <meta name="twitter:card" content="summary">
  <meta name="twitter:title" content="%s">
  <meta name="twitter:description" content="%s">
</head>
<body>
  <h1>%s</h1>
  <p>%s</p>
  <p><a href="/">Return to maek</a></p>
</body>
</html>
`, desc, title, desc, title, desc)
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
	// 1. Direct service URL access: /_maek/<service-name-or-id>[/subpath]
	if strings.HasPrefix(r.URL.Path, "/_maek/") {
		trimmed := strings.TrimPrefix(r.URL.Path, "/_maek/")
		parts := strings.SplitN(trimmed, "/", 2)
		rawKey := parts[0]

		if rawKey != "" && !protocol.IsReservedName(rawKey) {
			serviceKey, err := url.PathUnescape(rawKey)
			if err != nil {
				serviceKey = rawKey
			}
			serviceKey = strings.TrimSpace(serviceKey)

			session, ok := s.registry.Get(serviceKey)
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprintf(w, "Service '%s' is not found or has disconnected. <a href=\"/\">Return to Catalog</a>", serviceKey)
				return
			}

			// OpenGraph crawler detection: If request is from social media bots,
			// serve rich OpenGraph meta HTML without redirecting
			if isBot(r.UserAgent()) {
				s.serveOpenGraphHTML(w, r, session)
				return
			}

			subPath := "/"
			if len(parts) > 1 && parts[1] != "" {
				subPath = "/" + parts[1]
			}
			if r.URL.RawQuery != "" {
				subPath += "?" + r.URL.RawQuery
			}

			// Issue routing cookie and redirect to target subpath
			http.SetCookie(w, &http.Cookie{
				Name:     protocol.CookieService,
				Value:    session.Info.ID,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})

			http.Redirect(w, r, subPath, http.StatusFound)
			return
		}
	}

	// 2. Check custom header (priority for CLI / API)
	serviceKey := strings.TrimSpace(r.Header.Get(protocol.HeaderService))

	// 3. Check session cookie
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
