package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"sort"
	"sync"
	"time"

	"github.com/gosuda/maek/internal/protocol"
	"github.com/hashicorp/yamux"
)

var (
	ErrServiceNotFound = errors.New("service not found")
	ErrServiceExists   = errors.New("service already exists")
)

const reservationTTL = 30 * time.Second

// ServiceSession represents an active service and its tunnel runtime.
type ServiceSession struct {
	Info         protocol.ServiceInfo
	Session      *yamux.Session
	ReverseProxy *httputil.ReverseProxy
	generation   uint64
}

type pendingReservation struct {
	generation uint64
	expiresAt  time.Time
}

// Registry owns service identity and lifecycle state. ID allocation is a
// reservation, so a successful availability check cannot race with another
// handshake before registration commits.
type Registry struct {
	mu         sync.RWMutex
	services   map[string]*ServiceSession // keyed by ID
	pending    map[string]pendingReservation
	generation uint64
}

func NewRegistry() *Registry {
	return &Registry{
		services: make(map[string]*ServiceSession),
		pending:  make(map[string]pendingReservation),
	}
}

// Reservation is an ID lease held while a handshake is in progress.
type Reservation struct {
	registry   *Registry
	id         string
	generation uint64
	once       sync.Once
}

func (r *Reservation) ID() string { return r.id }

// Release gives up an uncommitted reservation. It is safe to call after the
// reservation has already been claimed by Register/Activate.
func (r *Reservation) Release() {
	if r == nil || r.registry == nil {
		return
	}
	r.once.Do(func() {
		r.registry.mu.Lock()
		defer r.registry.mu.Unlock()
		if pending, ok := r.registry.pending[r.id]; ok && pending.generation == r.generation {
			delete(r.registry.pending, r.id)
		}
	})
}

// Registration is a generation-scoped handle. Closing an old registration
// can never remove a newer service that later reuses the same ID.
type Registration struct {
	registry   *Registry
	id         string
	generation uint64
	once       sync.Once
}

func (r *Registration) Close() {
	if r == nil || r.registry == nil {
		return
	}
	r.once.Do(func() {
		r.registry.mu.Lock()
		defer r.registry.mu.Unlock()
		if current, ok := r.registry.services[r.id]; ok && current.generation == r.generation {
			delete(r.registry.services, r.id)
		}
	})
}

func (r *Registry) pruneExpiredLocked(now time.Time) {
	for id, pending := range r.pending {
		if !pending.expiresAt.After(now) {
			delete(r.pending, id)
		}
	}
}

func (r *Registry) idTakenLocked(id string) bool {
	if _, ok := r.services[id]; ok {
		return true
	}
	_, ok := r.pending[id]
	return ok
}

func (r *Registry) allocateIDLocked(preferred string) (string, error) {
	clean := protocol.SanitizePreferredID(preferred)
	if protocol.IsReservedName(clean) {
		clean = "app-" + clean
	}
	if clean == "" {
		for {
			id, err := protocol.GenerateID()
			if err != nil {
				return "", err
			}
			if !r.idTakenLocked(id) && !protocol.IsReservedName(id) {
				return id, nil
			}
		}
	}
	if !r.idTakenLocked(clean) {
		return clean, nil
	}
	for counter := 2; ; counter++ {
		suffix := fmt.Sprintf("-%d", counter)
		base := clean
		if len(base)+len(suffix) > protocol.MaxIDLength {
			base = base[:protocol.MaxIDLength-len(suffix)]
		}
		candidate := base + suffix
		if !r.idTakenLocked(candidate) {
			return candidate, nil
		}
	}
}

// ReserveID atomically chooses and reserves an ID until it is activated or
// released by the handshake owner.
func (r *Registry) ReserveID(preferred string) (*Reservation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	r.pruneExpiredLocked(now)
	id, err := r.allocateIDLocked(preferred)
	if err != nil {
		return nil, err
	}
	r.generation++
	generation := r.generation
	r.pending[id] = pendingReservation{generation: generation, expiresAt: now.Add(reservationTTL)}
	return &Reservation{registry: r, id: id, generation: generation}, nil
}

// AllocateID is retained for the current handshake while callers migrate to
// explicit Reservation ownership. The reservation is reclaimed on Register or
// lazily expires if an old caller abandons the handshake.
func (r *Registry) AllocateID(preferred string) (string, error) {
	reservation, err := r.ReserveID(preferred)
	if err != nil {
		return "", err
	}
	return reservation.ID(), nil
}

func buildServiceSession(info protocol.ServiceInfo, session *yamux.Session, modifyResponse func(*http.Response) error, generation uint64) *ServiceSession {
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
			if req.Header.Get("Accept-Encoding") != "" {
				req.Header.Set("Accept-Encoding", "gzip")
			}
		},
		Transport:      transport,
		ModifyResponse: modifyResponse,
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("502 Bad Gateway - maek could not reach agent: " + err.Error()))
		},
	}
	return &ServiceSession{Info: info, Session: session, ReverseProxy: proxy, generation: generation}
}

// Register claims a pending ID reservation when one exists. Direct test/setup
// callers without a prior reservation are still accepted, but duplicate IDs
// are never overwritten.
func (r *Registry) Register(info protocol.ServiceInfo, session *yamux.Session, modifyResponse func(*http.Response) error) (*ServiceSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.services[info.ID]; exists {
		return nil, ErrServiceExists
	}

	generation := uint64(0)
	if pending, ok := r.pending[info.ID]; ok {
		generation = pending.generation
		delete(r.pending, info.ID)
	} else {
		r.generation++
		generation = r.generation
	}

	ss := buildServiceSession(info, session, modifyResponse, generation)
	r.services[info.ID] = ss
	return ss, nil
}

// Activate commits an explicit reservation and returns a generation-scoped
// registration handle for safe cleanup.
func (r *Registry) Activate(reservation *Reservation, info protocol.ServiceInfo, session *yamux.Session, modifyResponse func(*http.Response) error) (*ServiceSession, *Registration, error) {
	if reservation == nil || reservation.registry != r || reservation.id != info.ID {
		return nil, nil, fmt.Errorf("invalid reservation")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	pending, ok := r.pending[reservation.id]
	if !ok || pending.generation != reservation.generation {
		return nil, nil, fmt.Errorf("reservation is no longer active")
	}
	if _, exists := r.services[info.ID]; exists {
		return nil, nil, ErrServiceExists
	}
	delete(r.pending, reservation.id)

	ss := buildServiceSession(info, session, modifyResponse, reservation.generation)
	r.services[info.ID] = ss
	registration := &Registration{registry: r, id: info.ID, generation: reservation.generation}
	return ss, registration, nil
}

// Unregister is the legacy ID-scoped cleanup path. New session code should
// hold Registration and call Close so cleanup is generation safe.
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ss, ok := r.services[id]; ok {
		delete(r.services, id)
		if ss.Session != nil && !ss.Session.IsClosed() {
			_ = ss.Session.Close()
		}
	}
}

// Get resolves an exact ID first, then a duplicate-friendly service name. The
// oldest active service wins name resolution, so disconnecting it naturally
// exposes the next-oldest service without maintaining a fragile reverse map.
func (r *Registry) Get(idOrName string) (*ServiceSession, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if ss, ok := r.services[idOrName]; ok {
		return ss, true
	}

	var selected *ServiceSession
	for _, ss := range r.services {
		if ss.Info.Name != idOrName {
			continue
		}
		if selected == nil || ss.Info.ConnectedAt.Before(selected.Info.ConnectedAt) ||
			(ss.Info.ConnectedAt.Equal(selected.Info.ConnectedAt) && ss.generation < selected.generation) {
			selected = ss
		}
	}
	return selected, selected != nil
}

// List returns a stable snapshot of all active services.
func (r *Registry) List() []protocol.ServiceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]protocol.ServiceInfo, 0, len(r.services))
	for _, ss := range r.services {
		list = append(list, ss.Info)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].ConnectedAt.Equal(list[j].ConnectedAt) {
			return list[i].ID < list[j].ID
		}
		return list[i].ConnectedAt.Before(list[j].ConnectedAt)
	})
	return list
}
