package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gosuda/maek/internal/protocol"
	"github.com/gosuda/maek/internal/version"
)

type Config struct {
	Addr string
}

type Server struct {
	addr     string
	registry *Registry
}

func NewServer(cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	return &Server{addr: cfg.Addr, registry: NewRegistry()}
}

func (s *Server) Registry() *Registry { return s.registry }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(protocol.EndpointWS, s.handleAgentWebSocket)
	mux.HandleFunc(protocol.EndpointFloatJS, s.handleFloatJS)
	mux.HandleFunc(protocol.EndpointServices, s.handleServicesAPI)
	mux.HandleFunc(protocol.EndpointVersion, s.handleVersion)
	mux.HandleFunc(protocol.EndpointThumb, s.handleThumb)
	mux.HandleFunc(protocol.EndpointSelect, s.handleSelectService)
	mux.HandleFunc(protocol.EndpointExit, s.handleExitService)
	mux.HandleFunc(protocol.EndpointInstallSh, s.handleInstallScript)
	mux.HandleFunc(protocol.EndpointInstallPs1, s.handleInstallScript)
	mux.HandleFunc(protocol.EndpointLLMsTxt, s.handleLLMsTxt)
	mux.HandleFunc(protocol.EndpointStyleCSS, s.handleStyleCSS)
	mux.HandleFunc(protocol.EndpointAppJS, s.handleAppJS)
	mux.HandleFunc(protocol.EndpointDownload, s.handleDownload)
	mux.HandleFunc("/", s.handleProxyOrCatalog)
	return mux
}

// Run owns the HTTP server lifecycle. No mutable http.Server pointer is shared
// between Start and Shutdown goroutines.
func (s *Server) Run(ctx context.Context) error {
	httpSrv := &http.Server{Addr: s.addr, Handler: s.Handler()}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("[maek-server] listening on http://%s", s.addr)
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return ctx.Err()
	}
}

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

func (s *Server) handleServicesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.registry.List())
}

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

func (s *Server) handleThumb(w http.ResponseWriter, r *http.Request) {
	idOrAlias := strings.TrimSpace(r.URL.Query().Get("id"))
	if idOrAlias == "" {
		http.NotFound(w, r)
		return
	}
	entry, ok := s.registry.Get(idOrAlias)
	if !ok || entry.Info.Thumbnail == "" {
		http.NotFound(w, r)
		return
	}
	thumb := entry.Info.Thumbnail
	if strings.HasPrefix(thumb, "data:") {
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

func (s *Server) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	filename := "install.sh"
	contentType := "text/x-shellscript; charset=utf-8"
	if strings.HasSuffix(r.URL.Path, ".ps1") {
		filename = "install.ps1"
		contentType = "text/plain; charset=utf-8"
	}
	data, err := GetStaticFile(filename)
	if err != nil {
		http.Error(w, "Installer script not found", http.StatusNotFound)
		return
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	serverURL := fmt.Sprintf("%s://%s", scheme, r.Host)
	content := strings.ReplaceAll(string(data), "__SERVER_URL__", serverURL)
	content = strings.ReplaceAll(content, "__VERSION__", version.Version)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(content))
}

func (s *Server) handleLLMsTxt(w http.ResponseWriter, r *http.Request) {
	data, err := GetStaticFile("llms.txt")
	if err != nil {
		http.Error(w, "llms.txt not found", http.StatusNotFound)
		return
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	serverURL := fmt.Sprintf("%s://%s", scheme, r.Host)
	content := strings.ReplaceAll(string(data), "__SERVER_URL__", serverURL)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(content))
}

func (s *Server) serveStaticAsset(w http.ResponseWriter, r *http.Request, name, contentType string) {
	data, err := GetStaticFile(name)
	if err != nil {
		http.Error(w, "asset not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func (s *Server) handleStyleCSS(w http.ResponseWriter, r *http.Request) {
	s.serveStaticAsset(w, r, "style.css", "text/css; charset=utf-8")
}

func (s *Server) handleAppJS(w http.ResponseWriter, r *http.Request) {
	s.serveStaticAsset(w, r, "app.js", "application/javascript; charset=utf-8")
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	osName := strings.TrimSpace(r.URL.Query().Get("os"))
	archName := strings.TrimSpace(r.URL.Query().Get("arch"))
	switch strings.ToLower(osName) {
	case "darwin", "mac", "macos", "osx":
		osName = "Darwin"
	case "windows", "win":
		osName = "Windows"
	default:
		osName = "Linux"
	}
	switch strings.ToLower(archName) {
	case "arm64", "aarch64":
		archName = "arm64"
	default:
		archName = "x86_64"
	}
	var filename, contentType string
	if osName == "Windows" {
		filename = "maek_Windows_x86_64.zip"
		contentType = "application/zip"
	} else {
		filename = fmt.Sprintf("maek_%s_%s.tar.gz", osName, archName)
		contentType = "application/gzip"
	}
	data, err := GetStaticFile("bin/" + filename)
	if err == nil && len(data) > 0 {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(data)
		return
	}
	ver := version.Version
	if ver == "" || ver == "dev" {
		ver = "0.1.0"
	}
	ver = strings.TrimPrefix(ver, "v")
	releaseFilename := fmt.Sprintf("maek_%s_%s_%s", ver, osName, archName)
	if osName == "Windows" {
		releaseFilename = fmt.Sprintf("maek_%s_Windows_x86_64.zip", ver)
	} else {
		releaseFilename += ".tar.gz"
	}
	githubURL := fmt.Sprintf("https://github.com/gosuda/maek/releases/download/v%s/%s", ver, releaseFilename)
	http.Redirect(w, r, githubURL, http.StatusFound)
}

func isBot(ua string) bool {
	if ua == "" {
		return false
	}
	ua = strings.ToLower(ua)
	for _, sig := range []string{
		"bot", "crawler", "spider", "scraper", "facebookexternalhit", "twitterbot", "slackbot",
		"discordbot", "telegrambot", "kakaotalk-scrap", "whatsapp", "linkedinbot", "meta-externalagent",
		"applebot", "bingbot", "googlebot", "yandex", "embedly", "quora link preview", "showyoubot",
		"outbrain", "pinterest", "vkshare",
	} {
		if strings.Contains(ua, sig) {
			return true
		}
	}
	return false
}

func (s *Server) serveOpenGraphHTML(w http.ResponseWriter, r *http.Request, entry *ServiceSession) {
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
	if entry.Info.Thumbnail != "" {
		if strings.HasPrefix(entry.Info.Thumbnail, "http://") || strings.HasPrefix(entry.Info.Thumbnail, "https://") {
			imageURL = entry.Info.Thumbnail
		} else {
			imageURL = fmt.Sprintf("%s://%s%s?id=%s", scheme, host, protocol.EndpointThumb, entry.Info.ID)
		}
	}
	title := html.EscapeString(entry.Info.Alias)
	desc := html.EscapeString(entry.Info.Description)
	if desc == "" {
		desc = fmt.Sprintf("Tunnel to %s hosted via maek", entry.Info.Alias)
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
	http.SetCookie(w, &http.Cookie{Name: protocol.CookieService, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleExitService(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: protocol.CookieService, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleProxyOrCatalog(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/_maek/") {
		trimmed := strings.TrimPrefix(r.URL.Path, "/_maek/")
		parts := strings.SplitN(trimmed, "/", 2)
		rawKey := parts[0]
		if rawKey != "" && !protocol.IsReservedHandle(rawKey) {
			serviceKey, err := url.PathUnescape(rawKey)
			if err != nil {
				serviceKey = rawKey
			}
			serviceKey = strings.TrimSpace(serviceKey)
			entry, ok := s.registry.Get(serviceKey)
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprintf(w, "Service '%s' is not found or has disconnected. <a href=\"/\">Return to Catalog</a>", serviceKey)
				return
			}
			if isBot(r.UserAgent()) {
				s.serveOpenGraphHTML(w, r, entry)
				return
			}
			subPath := "/"
			if len(parts) > 1 && parts[1] != "" {
				subPath = "/" + parts[1]
			}
			if r.URL.RawQuery != "" {
				subPath += "?" + r.URL.RawQuery
			}
			http.SetCookie(w, &http.Cookie{Name: protocol.CookieService, Value: entry.Info.ID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
			http.Redirect(w, r, subPath, http.StatusFound)
			return
		}
	}

	serviceKey := strings.TrimSpace(r.Header.Get(protocol.HeaderService))
	if serviceKey == "" {
		if cookie, err := r.Cookie(protocol.CookieService); err == nil {
			serviceKey = strings.TrimSpace(cookie.Value)
		}
	}
	if serviceKey == "" {
		if r.URL.Path == "/" {
			s.serveCatalogUI(w, r)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	entry, ok := s.registry.Get(serviceKey)
	if !ok {
		http.SetCookie(w, &http.Cookie{Name: protocol.CookieService, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Service '%s' is not found or has disconnected. <a href=\"/\">Return to Catalog</a>", serviceKey)
		return
	}
	if entry.ReverseProxy == nil {
		http.Error(w, "Service tunnel is not ready", http.StatusBadGateway)
		return
	}
	entry.ReverseProxy.ServeHTTP(w, r)
}

func (s *Server) serveCatalogUI(w http.ResponseWriter, r *http.Request) {
	data, err := GetStaticFile("index.html")
	if err != nil {
		http.Error(w, "Web UI not available", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	_, _ = w.Write(data)
}
