package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"sync"
	"time"

	"github.com/gosuda/maek/internal/protocol"
	"github.com/hashicorp/yamux"
)

var (
	ErrServiceNotFound = errors.New("service not found")
	ErrServiceExists   = errors.New("service already exists")
)

// ServiceSession represents a connected agent and its yamux tunnel.
type ServiceSession struct {
	Info         protocol.ServiceInfo
	Session      *yamux.Session
	ReverseProxy *httputil.ReverseProxy
}

// Registry manages all active agent sessions.
type Registry struct {
	mu       sync.RWMutex
	services map[string]*ServiceSession // keyed by ID
	byName   map[string]string          // maps Name -> ID
}

func NewRegistry() *Registry {
	return &Registry{
		services: make(map[string]*ServiceSession),
		byName:   make(map[string]string),
	}
}

// Register adds an active agent session to the registry.
func (r *Registry) Register(info protocol.ServiceInfo, session *yamux.Session, modifyResponse func(*http.Response) error) (*ServiceSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create custom transport that dials streams through this yamux session
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return session.Open()
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "maek"

			// Only allow gzip or uncompressed from upstream so HTML injection works reliably
			if req.Header.Get("Accept-Encoding") != "" {
				req.Header.Set("Accept-Encoding", "gzip")
			}
		},
		Transport:      transport,
		ModifyResponse: modifyResponse,
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("502 Bad Gateway - maek could not reach agent: " + err.Error()))
		},
	}

	ss := &ServiceSession{
		Info:         info,
		Session:      session,
		ReverseProxy: proxy,
	}

	r.services[info.ID] = ss
	r.byName[info.Name] = info.ID
	return ss, nil
}

// Unregister removes a service session.
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ss, ok := r.services[id]; ok {
		delete(r.services, id)
		if r.byName[ss.Info.Name] == id {
			delete(r.byName, ss.Info.Name)
		}
		if ss.Session != nil && !ss.Session.IsClosed() {
			ss.Session.Close()
		}
	}
}

// Get retrieves a service by its 6-character ID or by its service Name.
func (r *Registry) Get(idOrName string) (*ServiceSession, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. Exact ID match
	if ss, ok := r.services[idOrName]; ok {
		return ss, true
	}

	// 2. Name match
	if id, ok := r.byName[idOrName]; ok {
		if ss, ok := r.services[id]; ok {
			return ss, true
		}
	}

	return nil, false
}

// List returns a snapshot of all active services.
func (r *Registry) List() []protocol.ServiceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]protocol.ServiceInfo, 0, len(r.services))
	for _, ss := range r.services {
		list = append(list, ss.Info)
	}
	return list
}
