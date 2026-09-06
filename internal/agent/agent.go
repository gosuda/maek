package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/gosuda/maek/internal/protocol"
	"github.com/hashicorp/yamux"
)

type Config struct {
	ServerURL string // e.g. "ws://localhost:8080" or "http://localhost:8080"
	Name      string // e.g. "my-app"
	Target    string // e.g. "http://localhost:3000"
}

type Agent struct {
	cfg        Config
	targetURL  *url.URL
	targetAddr string
	isTLS      bool

	stopChan chan struct{}
	wg       sync.WaitGroup
}

func NewAgent(cfg Config) (*Agent, error) {
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	if cfg.Name == "" {
		cfg.Name = "app"
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
	hostPort := parsedTarget.Host
	if !strings.Contains(hostPort, ":") {
		if isTLS {
			hostPort += ":443"
		} else {
			hostPort += ":80"
		}
	}

	return &Agent{
		cfg:        cfg,
		targetURL:  parsedTarget,
		targetAddr: hostPort,
		isTLS:      isTLS,
		stopChan:   make(chan struct{}),
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

// Stop stops the agent.
func (a *Agent) Stop() {
	close(a.stopChan)
	a.wg.Wait()
}

// buildWebSocketURL converts http(s) to ws(s) and appends /_maek/ws with query params.
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

	q := url.Values{}
	q.Set("name", a.cfg.Name)
	q.Set("target", a.cfg.Target)

	return srv + "?" + q.Encode()
}

func (a *Agent) connectAndServe(ctx context.Context) error {
	wsURL := a.buildWebSocketURL()
	log.Printf("[maek-agent] Connecting to %s...", wsURL)

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	wsConn, resp, err := websocket.Dial(dialCtx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}
	defer wsConn.Close(websocket.StatusNormalClosure, "agent stopping")

	serviceID := ""
	if resp != nil && resp.Header != nil {
		serviceID = resp.Header.Get(protocol.HeaderMaekID)
	}

	log.Printf("[maek-agent] Connected to server! Assigned Service ID: [%s], Name: '%s', Target: %s",
		serviceID, a.cfg.Name, a.cfg.Target)

	netConn := websocket.NetConn(ctx, wsConn, websocket.MessageBinary)

	yamuxCfg := yamux.DefaultConfig()
	yamuxCfg.LogOutput = io.Discard
	session, err := yamux.Client(netConn, yamuxCfg)
	if err != nil {
		return fmt.Errorf("yamux client init failed: %w", err)
	}
	defer session.Close()

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			return fmt.Errorf("yamux accept stream: %w", err)
		}

		a.wg.Add(1)
		go func(s net.Conn) {
			defer a.wg.Done()
			a.handleStream(s)
		}(stream)
	}
}

// handleStream bridges the incoming Yamux virtual stream with the local target server.
func (a *Agent) handleStream(stream net.Conn) {
	defer stream.Close()

	var targetConn net.Conn
	var err error

	if a.isTLS {
		targetConn, err = tls.Dial("tcp", a.targetAddr, &tls.Config{
			InsecureSkipVerify: true,
		})
	} else {
		targetConn, err = net.DialTimeout("tcp", a.targetAddr, 5*time.Second)
	}

	if err != nil {
		log.Printf("[maek-agent] Failed to dial local target %s: %v", a.targetAddr, err)
		errMsg := fmt.Sprintf("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain; charset=utf-8\r\nConnection: close\r\n\r\nmaek-agent could not connect to local target: %v\r\n", err)
		_, _ = stream.Write([]byte(errMsg))
		return
	}
	defer targetConn.Close()

	// Bidirectional pipe
	errChan := make(chan error, 2)
	go func() {
		_, copyErr := io.Copy(targetConn, stream)
		errChan <- copyErr
	}()
	go func() {
		_, copyErr := io.Copy(stream, targetConn)
		errChan <- copyErr
	}()

	// Wait until one direction finishes
	<-errChan
}
