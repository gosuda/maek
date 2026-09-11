package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/gosuda/maek/internal/protocol"
	"github.com/gosuda/maek/internal/tunnel"
	"github.com/gosuda/maek/internal/version"
)

type ServiceConfig struct {
	Alias       string
	Description string
	Thumbnail   string
	Target      string
}

type Config struct {
	ServerURL string
	Services  []ServiceConfig
}

type localService struct {
	cfg       ServiceConfig
	targetURL *url.URL
	proxy     *httputil.ReverseProxy
}

type Agent struct {
	cfg      Config
	services []localService
}

func NewAgent(cfg Config) (*Agent, error) {
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	if len(cfg.Services) == 0 {
		return nil, fmt.Errorf("at least one service is required")
	}

	services := make([]localService, 0, len(cfg.Services))
	for i := range cfg.Services {
		svc := cfg.Services[i]
		if svc.Target == "" {
			svc.Target = "http://localhost:8080"
		}
		target := svc.Target
		if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			target = "http://" + target
		}
		parsedTarget, err := url.Parse(target)
		if err != nil {
			return nil, fmt.Errorf("service %d target URL: %w", i, err)
		}

		if svc.Alias == "" || svc.Description == "" || svc.Thumbnail == "" {
			scrapedAlias, scrapedDesc, scrapedThumb := ScrapeTargetMetadata(parsedTarget)
			if svc.Alias == "" && scrapedAlias != "" {
				svc.Alias = scrapedAlias
				log.Printf("[maek-agent] auto-detected service alias from target: %q", scrapedAlias)
			}
			if svc.Description == "" && scrapedDesc != "" {
				svc.Description = scrapedDesc
			}
			if svc.Thumbnail == "" && scrapedThumb != "" {
				svc.Thumbnail = scrapedThumb
			}
		}
		if svc.Alias == "" {
			svc.Alias = "app"
		}
		cfg.Services[i] = svc
		services = append(services, localService{cfg: svc, targetURL: parsedTarget, proxy: newTargetProxy(parsedTarget)})
	}

	return &Agent{cfg: cfg, services: services}, nil
}

func newTargetProxy(target *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = target.Host
		req.Header.Set("X-Forwarded-Host", target.Host)
		req.Header.Set("X-Forwarded-Proto", target.Scheme)
		if req.Header.Get("Origin") != "" {
			req.Header.Set("Origin", target.Scheme+"://"+target.Host)
		}
		if ref := req.Header.Get("Referer"); ref != "" {
			if parsedRef, err := url.Parse(ref); err == nil {
				parsedRef.Scheme = target.Scheme
				parsedRef.Host = target.Host
				req.Header.Set("Referer", parsedRef.String())
			}
		}
	}
	if target.Scheme == "https" {
		proxy.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	return proxy
}

func (a *Agent) Config() Config { return a.cfg }

func (a *Agent) Run(ctx context.Context) error {
	backoff := time.Second
	const maxBackoff = 15 * time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := a.connectAndServe(ctx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			log.Printf("[maek-agent] connection lost (%v); reconnecting in %v", err, backoff)
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

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
	return srv
}

type serviceIDContextKey struct{}

func (a *Agent) connectAndServe(ctx context.Context) error {
	wsURL := a.buildWebSocketURL()
	log.Printf("[maek-agent] connecting to %s", wsURL)

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	wsConn, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{Subprotocols: protocol.SupportedSubprotocols()})
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}
	defer wsConn.Close(websocket.StatusNormalClosure, "agent stopping")

	hsCtx, hsCancel := context.WithTimeout(ctx, 15*time.Second)
	defer hsCancel()
	sessionConfig, err := tunnel.ClientNegotiate(hsCtx, wsConn, version.Version)
	if err != nil {
		return fmt.Errorf("protocol negotiation failed: %w", err)
	}

	register := protocol.RegisterServices{Services: make([]protocol.ServiceSpec, 0, len(a.services))}
	for _, svc := range a.services {
		register.Services = append(register.Services, protocol.ServiceSpec{
			Alias:       svc.cfg.Alias,
			Description: svc.cfg.Description,
			Thumbnail:   svc.cfg.Thumbnail,
		})
	}
	if err := tunnel.WriteControl(hsCtx, wsConn, protocol.MsgRegisterV1, register); err != nil {
		return fmt.Errorf("register send failed: %w", err)
	}
	env, err := tunnel.ReadControl(hsCtx, wsConn)
	if err != nil {
		return fmt.Errorf("register response failed: %w", err)
	}
	if env.Type == protocol.MsgErrorV1 {
		failure, decodeErr := protocol.DecodePayload[protocol.ProtocolError](env)
		if decodeErr != nil {
			return decodeErr
		}
		return fmt.Errorf("server rejected registration: %s (%s)", failure.Message, failure.Code)
	}
	if env.Type != protocol.MsgRegistered {
		return fmt.Errorf("unexpected register response %q", env.Type)
	}
	registered, err := protocol.DecodePayload[protocol.RegisteredServices](env)
	if err != nil {
		return err
	}
	if len(registered.Services) != len(a.services) {
		return fmt.Errorf("server registered %d services, want %d", len(registered.Services), len(a.services))
	}

	proxies := make(map[string]*httputil.ReverseProxy, len(registered.Services))
	for _, assigned := range registered.Services {
		if int(assigned.Index) >= len(a.services) {
			return fmt.Errorf("server returned invalid service index %d", assigned.Index)
		}
		proxies[assigned.ID] = a.services[assigned.Index].proxy
		log.Printf("[maek-agent] registered alias %q as [%s] -> %s", assigned.Alias, assigned.ID, a.services[assigned.Index].cfg.Target)
	}
	if err := tunnel.WriteControl(hsCtx, wsConn, protocol.MsgStartV1, struct{}{}); err != nil {
		return fmt.Errorf("start send failed: %w", err)
	}

	netConn := websocket.NetConn(ctx, wsConn, websocket.MessageBinary)
	tunnelSession, err := tunnel.NewClient(netConn, sessionConfig)
	if err != nil {
		return fmt.Errorf("tunnel session init failed: %w", err)
	}
	defer tunnelSession.Close()

	localSrv := &http.Server{
		ConnContext: func(parent context.Context, conn net.Conn) context.Context {
			if id, ok := tunnel.ServiceID(conn); ok {
				return context.WithValue(parent, serviceIDContextKey{}, id)
			}
			return parent
		},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := r.Context().Value(serviceIDContextKey{}).(string)
			proxy, ok := proxies[id]
			if !ok {
				http.Error(w, "unknown maek service", http.StatusBadGateway)
				return
			}
			proxy.ServeHTTP(w, r)
		}),
	}

	errCh := make(chan error, 1)
	go func() { errCh <- localSrv.Serve(tunnelSession.Listener()) }()
	select {
	case <-ctx.Done():
		_ = localSrv.Close()
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
