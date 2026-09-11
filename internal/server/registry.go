package server

import (
	"errors"
	"fmt"
	"net/http/httputil"
	"sort"
	"strings"
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

func aliasIDBase(alias string) string {
	alias = strings.TrimSpace(strings.ToLower(alias))
	var b strings.Builder
	b.Grow(len(alias))
	separator := false
	for _, ch := range alias {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9':
			if separator && b.Len() > 0 && b.Len() < protocol.MaxServiceIDLength {
				b.WriteByte('-')
			}
			separator = false
			if b.Len() < protocol.MaxServiceIDLength {
				b.WriteRune(ch)
			}
		case ch == '-' || ch == '_':
			separator = b.Len() > 0
		default:
			separator = b.Len() > 0
		}
		if b.Len() >= protocol.MaxServiceIDLength {
			break
		}
	}
	base := strings.Trim(b.String(), "-_")
	if base == "" {
		return ""
	}
	if protocol.IsReservedHandle(base) {
		prefix := "app-"
		maxBase := protocol.MaxServiceIDLength - len(prefix)
		if len(base) > maxBase {
			base = base[:maxBase]
		}
		base = prefix + base
	}
	return base
}

func (r *Registry) randomIDLocked() (string, error) {
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

func (r *Registry) allocateIDLocked(alias string) (string, error) {
	base := aliasIDBase(alias)
	if base == "" {
		return r.randomIDLocked()
	}
	if !r.idTakenLocked(base) {
		return base, nil
	}
	for n := 2; ; n++ {
		suffix := fmt.Sprintf("-%d", n)
		trimmed := base
		maxBase := protocol.MaxServiceIDLength - len(suffix)
		if len(trimmed) > maxBase {
			trimmed = strings.TrimRight(trimmed[:maxBase], "-_")
		}
		if trimmed == "" {
			return r.randomIDLocked()
		}
		candidate := trimmed + suffix
		if !r.idTakenLocked(candidate) {
			return candidate, nil
		}
	}
}

// ReserveID derives a stable ID base from Alias and atomically reserves a
// unique variant. Duplicate aliases receive -2, -3, ... suffixes. If Alias
// cannot produce a usable ID, a random server-owned ID is used instead.
func (r *Registry) ReserveID(alias string) (*Reservation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id, err := r.allocateIDLocked(alias)
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
