package server

import (
	"sync"
	"testing"
	"time"

	"github.com/gosuda/maek/internal/protocol"
	"github.com/gosuda/maek/internal/service"
)

func activateTestService(t *testing.T, r *Registry, alias string, connectedAt time.Time) (*Registration, string) {
	t.Helper()
	reservation, err := r.ReserveID()
	if err != nil {
		t.Fatal(err)
	}
	id := reservation.ID()
	_, registration, err := r.Activate(reservation, service.Info{ID: id, Alias: alias, ConnectedAt: connectedAt}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return registration, id
}

func TestRegistryReservationGeneratesOpaqueID(t *testing.T) {
	r := NewRegistry()
	reservation, err := r.ReserveID()
	if err != nil {
		t.Fatal(err)
	}
	defer reservation.Release()
	if len(reservation.ID()) != protocol.IDLength {
		t.Fatalf("got ID %q with length %d", reservation.ID(), len(reservation.ID()))
	}
}

func TestRegistryConcurrentReservationsAreUnique(t *testing.T) {
	const n = 64
	r := NewRegistry()
	reservations := make([]*Reservation, n)
	var wg sync.WaitGroup
	for i := range reservations {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			reservation, err := r.ReserveID()
			if err != nil {
				t.Errorf("reserve %d: %v", i, err)
				return
			}
			reservations[i] = reservation
		}(i)
	}
	wg.Wait()

	seen := make(map[string]struct{}, n)
	for _, reservation := range reservations {
		if reservation == nil {
			continue
		}
		if _, exists := seen[reservation.ID()]; exists {
			t.Fatalf("duplicate reservation %q", reservation.ID())
		}
		seen[reservation.ID()] = struct{}{}
		reservation.Release()
	}
	if len(seen) != n {
		t.Fatalf("got %d unique IDs, want %d", len(seen), n)
	}
}

func TestRegistryAliasOldestActiveWins(t *testing.T) {
	r := NewRegistry()
	now := time.Now()
	oldest, oldestID := activateTestService(t, r, "shared", now)
	newer, newerID := activateTestService(t, r, "shared", now.Add(time.Second))
	defer newer.Close()

	got, ok := r.Get("shared")
	if !ok || got.Info.ID != oldestID {
		t.Fatalf("got %+v, %v; want %s", got, ok, oldestID)
	}
	oldest.Close()
	got, ok = r.Get("shared")
	if !ok || got.Info.ID != newerID {
		t.Fatalf("got %+v, %v; want %s after oldest closes", got, ok, newerID)
	}
}

func TestRegistrationCloseIsGenerationScoped(t *testing.T) {
	r := NewRegistry()
	now := time.Now()
	first, id := activateTestService(t, r, "shared", now)

	// Simulate a rare ID reuse after the old runtime entry disappeared but
	// before its deferred Registration.Close executes.
	r.mu.Lock()
	delete(r.services, id)
	r.generation++
	generation := r.generation
	r.pending[id] = generation
	r.mu.Unlock()

	reservation := &Reservation{registry: r, id: id, generation: generation}
	_, second, err := r.Activate(reservation, service.Info{ID: id, Alias: "shared", ConnectedAt: now.Add(time.Second)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	first.Close()
	got, ok := r.Get(id)
	if !ok || got.Info.ID != id {
		t.Fatal("stale registration cleanup removed the new generation")
	}
}

func TestRegistryListSortedOldestFirst(t *testing.T) {
	r := NewRegistry()
	now := time.Now()
	newer, newerID := activateTestService(t, r, "new", now.Add(time.Second))
	older, olderID := activateTestService(t, r, "old", now)
	defer newer.Close()
	defer older.Close()
	list := r.List()
	if len(list) != 2 || list[0].ID != olderID || list[1].ID != newerID {
		t.Fatalf("unexpected order: %+v", list)
	}
}
