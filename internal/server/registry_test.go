package server

import (
	"sync"
	"testing"
	"time"

	"github.com/gosuda/maek/internal/service"
)

func activateTestService(t *testing.T, r *Registry, id, alias string, connectedAt time.Time) *Registration {
	t.Helper()
	reservation, err := r.ReserveID(id)
	if err != nil {
		t.Fatal(err)
	}
	_, registration, err := r.Activate(reservation, service.Info{ID: reservation.ID(), Alias: alias, ConnectedAt: connectedAt}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func TestRegistryReservationSuffix(t *testing.T) {
	r := NewRegistry()
	first, err := r.ReserveID("demo")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	second, err := r.ReserveID("demo")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()
	if first.ID() != "demo" || second.ID() != "demo-2" {
		t.Fatalf("got %q, %q", first.ID(), second.ID())
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
			reservation, err := r.ReserveID("demo")
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
	oldest := activateTestService(t, r, "owner-a", "shared", now)
	newer := activateTestService(t, r, "owner-b", "shared", now.Add(time.Second))
	defer newer.Close()

	got, ok := r.Get("shared")
	if !ok || got.Info.ID != "owner-a" {
		t.Fatalf("got %+v, %v; want owner-a", got, ok)
	}
	oldest.Close()
	got, ok = r.Get("shared")
	if !ok || got.Info.ID != "owner-b" {
		t.Fatalf("got %+v, %v; want owner-b after oldest closes", got, ok)
	}
}

func TestRegistrationCloseIsGenerationScoped(t *testing.T) {
	r := NewRegistry()
	now := time.Now()
	first := activateTestService(t, r, "demo", "shared", now)

	// Simulate runtime removal before its deferred Registration.Close executes.
	r.mu.Lock()
	delete(r.services, "demo")
	r.mu.Unlock()

	second := activateTestService(t, r, "demo", "shared", now.Add(time.Second))
	defer second.Close()
	first.Close()

	got, ok := r.Get("demo")
	if !ok || got.Info.ID != "demo" {
		t.Fatal("stale registration cleanup removed the new generation")
	}
}

func TestRegistryListSortedOldestFirst(t *testing.T) {
	r := NewRegistry()
	now := time.Now()
	newer := activateTestService(t, r, "new", "new", now.Add(time.Second))
	older := activateTestService(t, r, "old", "old", now)
	defer newer.Close()
	defer older.Close()
	list := r.List()
	if len(list) != 2 || list[0].ID != "old" || list[1].ID != "new" {
		t.Fatalf("unexpected order: %+v", list)
	}
}
