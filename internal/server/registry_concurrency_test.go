package server

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gosuda/maek/internal/protocol"
)

func TestRegistryConcurrentReservationsAreUnique(t *testing.T) {
	const n = 64
	r := NewRegistry()
	reservations := make([]*Reservation, n)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
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
			t.Fatalf("duplicate reservation for %q", reservation.ID())
		}
		seen[reservation.ID()] = struct{}{}
		reservation.Release()
	}
	if len(seen) != n {
		t.Fatalf("got %d unique ids, want %d", len(seen), n)
	}
}

func TestRegistryNameResolutionUsesOldestActive(t *testing.T) {
	r := NewRegistry()
	old := protocol.ServiceInfo{ID: "old", Name: "demo", ConnectedAt: time.Now()}
	newer := protocol.ServiceInfo{ID: "new", Name: "demo", ConnectedAt: old.ConnectedAt.Add(time.Second)}
	if _, err := r.Register(old, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Register(newer, nil, nil); err != nil {
		t.Fatal(err)
	}

	got, ok := r.Get("demo")
	if !ok || got.Info.ID != "old" {
		t.Fatalf("resolved %+v, %v; want old", got, ok)
	}
	r.Unregister("old")
	got, ok = r.Get("demo")
	if !ok || got.Info.ID != "new" {
		t.Fatalf("resolved %+v, %v; want new after oldest disconnects", got, ok)
	}
}

func TestRegistryRejectsDuplicateID(t *testing.T) {
	r := NewRegistry()
	info := protocol.ServiceInfo{ID: "demo", Name: "one", ConnectedAt: time.Now()}
	if _, err := r.Register(info, nil, nil); err != nil {
		t.Fatal(err)
	}
	info.Name = "two"
	if _, err := r.Register(info, nil, nil); !errors.Is(err, ErrServiceExists) {
		t.Fatalf("got %v, want ErrServiceExists", err)
	}
}

func TestRegistrationCloseCannotDeleteNewGeneration(t *testing.T) {
	r := NewRegistry()
	first, err := r.ReserveID("demo")
	if err != nil {
		t.Fatal(err)
	}
	_, oldRegistration, err := r.Activate(first, protocol.ServiceInfo{ID: first.ID(), Name: "demo", ConnectedAt: time.Now()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate the old runtime disappearing before its deferred cleanup runs.
	r.mu.Lock()
	delete(r.services, first.ID())
	r.mu.Unlock()

	second, err := r.ReserveID("demo")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.Activate(second, protocol.ServiceInfo{ID: second.ID(), Name: "demo", ConnectedAt: time.Now()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	oldRegistration.Close()
	if got, ok := r.Get(second.ID()); !ok || got.generation != second.generation {
		t.Fatalf("stale cleanup removed the new generation")
	}
}
