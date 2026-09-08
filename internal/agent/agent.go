package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/gosuda/maek/internal/protocol"
	"github.com/hashicorp/yamux"
)

type Config struct {
	ServerURL   string // e.g. "ws://localhost:8080" or "http://localhost:8080"
	Name        string // e.g. "my-app"
	PreferredID string // optional custom preferred ID (up to 32 chars)
	Description string // optional service description
	Thumbnail   string // optional thumbnail/avatar URL
	Target      string // local target, e.g. "http://localhost:3000" (kept private to agent)
}

type Agent struct {
	cfg       Config
	targetURL *url.URL
	isTLS     bool

	stopChan chan struct{}
	wg       sync.WaitGroup
}

func NewAgent(cfg Config) (*Agent, error) {
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	if cfg.Target == "" {
		cfg.Target = "http://localhost:8080"
	}

	// Normalize target URL
	target := cfg.Target
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}

	parsedTarget, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	isTLS := parsedTarget.Scheme == "https"

	// Auto-scrape metadata if name, description, or thumbnail is missing
	if cfg.Name == "" || cfg.Description == "" || cfg.Thumbnail == "" {
		scrapedName, scrapedDesc, scrapedThumb := ScrapeTargetMetadata(parsedTarget)
		if cfg.Name == "" && scrapedName != "" {
			cfg.Name = scrapedName
			log.Printf("[maek-agent] Auto-detected service name from target: %q", scrapedName)
		}
		if cfg.Description == "" && scrapedDesc != "" {
			cfg.Description = scrapedDesc
			log.Printf("[maek-agent] Auto-detected description from target: %q", scrapedDesc)
		}
		if cfg.Thumbnail == "" && scrapedThumb != "" {
			cfg.Thumbnail = scrapedThumb
			log.Printf("[maek-agent] Auto-detected icon/thumbnail from target")
		}
	}

	if cfg.Name == "" {
		cfg.Name = "app"
	}
	if cfg.PreferredID == "" {
		cfg.PreferredID = cfg.Name
	}

	return &Agent{
		cfg:       cfg,
		targetURL: parsedTarget,
		isTLS:     isTLS,
		stopChan:  make(chan struct{}),
	}, nil
}

// Start runs the agent with automatic reconnection until ctx is cancelled or Stop is called.
func (a *Agent) Start(ctx context.Context) error {
	backoff := 1 * time.Second
	maxBackoff := 15 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.stopChan:
			return nil
		default:
		}

		err := a.connectAndServe(ctx)
		if err != nil {
			log.Printf("[maek-agent] Connection lost (%v). Reconnecting in %v...", err, backoff)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.stopChan:
			return nil
		case <-time.After(backoff):
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// Config returns a copy of the agent configuration.
func (a *Agent) Config() Config {
	return a.cfg
}

// Stop stops the agent.
func (a *Agent) Stop() {
	close(a.stopChan)
	a.wg.Wait()
}

// buildWebSocketURL converts http(s) to ws(s) and appends /_maek/ws with metadata (no target!).
func (a *Agent) buildWebSocketURL() string {
	srv := a.cfg.ServerURL
	if strings.HasPrefix(srv, "http://") {
		srv = "ws://" + strings.TrimPrefix(srv, "http://")
	} else if strings.HasPrefix(srv, "https://") {
		srv = "wss://" + strings.TrimPrefix(srv, "https://")
	} else if !strings.HasPrefix(srv, "ws://") && !strings.HasPrefix(srv, "wss://") {
		srv = "ws://" + srv
	}

	srv = strings.TrimRight(srv, "/")
	if !strings.HasSuffix(srv, protocol.EndpointWS) {
		srv += protocol.EndpointWS
	}

	// Service metadata travels in control frames after the WebSocket is
	// established (protocol.AgentMessage), not in the URL.
	return srv
}

func (a *Agent) connectAndServe(ctx context.Context) error {
	wsURL := a.buildWebSocketURL()
	log.Printf("[maek-agent] Connecting to %s...", wsURL)

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	wsConn, _, err := websocket.Dial(dialCtx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}
	defer wsConn.Close(websocket.StatusNormalClosure, "agent stopping")

	// Control-message handshake: register -> registered -> start.
	hsCtx, hsCancel := context.WithTimeout(ctx, 15*time.Second)
	defer hsCancel()

	if err := protocol.WriteAgentMessage(hsCtx, wsConn, protocol.AgentMessage{
		Type: protocol.MsgTypeRegister,
		Service: &protocol.RegisterRequest{
			Name:        a.cfg.Name,
			ID:          a.cfg.PreferredID,
			Description: a.cfg.Description,
			Thumbnail:   a.cfg.Thumbnail,
		},
	}); err != nil {
		return fmt.Errorf("register send failed: %w", err)
	}

	ack, err := protocol.ReadAgentMessage(hsCtx, wsConn)
	if err != nil {
		return fmt.Errorf("handshake read failed: %w", err)
	}
	if ack.Type == protocol.MsgTypeError {
		return fmt.Errorf("server rejected registration: %s (%s)", ack.Message, ack.Code)
	}
	if ack.Type != protocol.MsgTypeRegistered || ack.ID == "" {
		return fmt.Errorf("unexpected handshake response: type=%q id=%q", ack.Type, ack.ID)
	}

	if err := protocol.WriteAgentMessage(hsCtx, wsConn, protocol.AgentMessage{
		Type: protocol.MsgTypeStart,
	}); err != nil {
		return fmt.Errorf("start send failed: %w", err)
	}

	serviceID := ack.ID
	serviceName := ack.Name
	if serviceName == "" {
		serviceName = a.cfg.Name
		log.Printf("[maek-agent] Warning: server did not return assigned Name; using requested name %q", serviceName)
	}
	log.Printf("[maek-agent] Connected to server! Assigned Service ID: [%s], Name: '%s' (forwarding to %s)",
		serviceID, serviceName, a.cfg.Target)

	netConn := websocket.NetConn(ctx, wsConn, websocket.MessageBinary)

	yamuxCfg := yamux.DefaultConfig()
	yamuxCfg.LogOutput = io.Discard
	session, err := yamux.Client(netConn, yamuxCfg)
	if err != nil {
		return fmt.Errorf("yamux client init failed: %w", err)
	}
	defer session.Close()

	// Local reverse proxy that speaks to private target
	targetProxy := httputil.NewSingleHostReverseProxy(a.targetURL)
	origDirector := targetProxy.Director
	targetProxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = a.targetURL.Host
		req.Header.Set("X-Forwarded-Host", a.targetURL.Host)
		req.Header.Set("X-Forwarded-Proto", a.targetURL.Scheme)

		// Fix Origin and Referer for CSWSH validation
		if req.Header.Get("Origin") != "" {
			req.Header.Set("Origin", a.targetURL.Scheme+"://"+a.targetURL.Host)
		}
		if ref := req.Header.Get("Referer"); ref != "" {
			if parsedRef, err := url.Parse(ref); err == nil {
				parsedRef.Scheme = a.targetURL.Scheme
				parsedRef.Host = a.targetURL.Host
				req.Header.Set("Referer", parsedRef.String())
			}
		}
	}

	if a.isTLS {
		targetProxy.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	localSrv := &http.Server{
		Handler: targetProxy,
	}

	srvErrCh := make(chan error, 1)
	go func() {
		srvErrCh <- localSrv.Serve(session)
	}()

	select {
	case <-ctx.Done():
		_ = localSrv.Close()
		return ctx.Err()
	case <-a.stopChan:
		_ = localSrv.Close()
		return nil
	case err := <-srvErrCh:
		return err
	}
}
