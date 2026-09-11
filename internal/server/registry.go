package server

import (
	"errors"
	"fmt"
	"net/http/httputil"
	"sort"
	"sync"

	"github.com/gosuda/maek/internal/protocol"
	"github.com/gosuda/maek/internal/service"
)

var (
	ErrServiceNotFound = errors.New("service not found")
	ErrServiceExists   = errors.New("service already exists")
)

// ServiceSession is the routable runtime state associated with an active ID.
// Registry stores it but does not create or close network resources.
type ServiceSession struct {
	Info         service.Info
	ReverseProxy *httputil.ReverseProxy
	generation   uint64
}

// Registry owns only service identity and lifecycle state.
type Registry struct {
	mu         sync.RWMutex
	services   map[string]*ServiceSession
	pending    map[string]uint64
	generation uint64
}

func NewRegistry() *Registry {
	return &Registry{
		services: make(map[string]*ServiceSession),
		pending:  make(map[string]uint64),
	}
}

type Reservation struct {
	registry   *Registry
	id         string
	generation uint64
	once       sync.Once
}

func (r *Reservation) ID() string { return r.id }

func (r *Reservation) Release() {
	if r == nil || r.registry == nil {
		return
	}
	r.once.Do(func() {
		r.registry.mu.Lock()
		defer r.registry.mu.Unlock()
		if generation, ok := r.registry.pending[r.id]; ok && generation == r.generation {
			delete(r.registry.pending, r.id)
		}
	})
}

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

func (r *Registry) idTakenLocked(id string) bool {
	if _, ok := r.services[id]; ok {
		return true
	}
	_, ok := r.pending[id]
	return ok
}

func (r *Registry) allocateIDLocked(preferred string) (string, error) {
	clean := protocol.SanitizePreferredID(preferred)
	if protocol.IsReservedHandle(clean) {
		clean = "app-" + clean
	}
	if clean == "" {
		for {
			id, err := protocol.GenerateID()
			if err != nil {
				return "", err
			}
			if !r.idTakenLocked(id) && !protocol.IsReservedHandle(id) {
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

func (r *Registry) ReserveID(preferred string) (*Reservation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id, err := r.allocateIDLocked(preferred)
	if err != nil {
		return nil, err
	}
	r.generation++
	generation := r.generation
	r.pending[id] = generation
	return &Reservation{registry: r, id: id, generation: generation}, nil
}

func (r *Registry) Activate(reservation *Reservation, info service.Info, proxy *httputil.ReverseProxy) (*ServiceSession, *Registration, error) {
	if reservation == nil || reservation.registry != r || reservation.id != info.ID {
		return nil, nil, fmt.Errorf("invalid reservation")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	generation, ok := r.pending[reservation.id]
	if !ok || generation != reservation.generation {
		return nil, nil, fmt.Errorf("reservation is no longer active")
	}
	if _, exists := r.services[info.ID]; exists {
		return nil, nil, ErrServiceExists
	}
	delete(r.pending, reservation.id)

	entry := &ServiceSession{Info: info, ReverseProxy: proxy, generation: reservation.generation}
	r.services[info.ID] = entry
	registration := &Registration{registry: r, id: info.ID, generation: reservation.generation}
	return entry, registration, nil
}

// Get resolves exact IDs first. Alias is intentionally non-unique; the oldest
// active registration wins and the next-oldest becomes visible when it exits.
func (r *Registry) Get(idOrAlias string) (*ServiceSession, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if entry, ok := r.services[idOrAlias]; ok {
		return entry, true
	}
	var selected *ServiceSession
	for _, entry := range r.services {
		if entry.Info.Alias != idOrAlias {
			continue
		}
		if selected == nil || entry.Info.ConnectedAt.Before(selected.Info.ConnectedAt) ||
			(entry.Info.ConnectedAt.Equal(selected.Info.ConnectedAt) && entry.generation < selected.generation) {
			selected = entry
		}
	}
	return selected, selected != nil
}

func (r *Registry) List() []service.Info {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]service.Info, 0, len(r.services))
	for _, entry := range r.services {
		list = append(list, entry.Info)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].ConnectedAt.Equal(list[j].ConnectedAt) {
			return list[i].ID < list[j].ID
		}
		return list[i].ConnectedAt.Before(list[j].ConnectedAt)
	})
	return list
}
