package server

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gosuda/maek/internal/protocol"
)

func TestRegistry_ResolveHandle_IDSuffix(t *testing.T) {
	r := NewRegistry()

	// 1. Empty preferred ID -> 6-char random
	id1, _, err := r.ReserveHandle("", "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id1) != protocol.IDLength {
		t.Fatalf("expected length %d, got %d (id: %s)", protocol.IDLength, len(id1), id1)
	}

	// 2. Unoccupied preferred ID
	id2, _, err := r.ReserveHandle("dev-code", "svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id2 != "dev-code" {
		t.Fatalf("expected dev-code, got %s", id2)
	}

	r.services["dev-code"] = &ServiceSession{}

	// 3. Occupied preferred ID -> should get dev-code-2
	id3, _, err := r.ReserveHandle("dev-code", "svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id3 != "dev-code-2" {
		t.Fatalf("expected dev-code-2, got %s", id3)
	}

	r.services["dev-code-2"] = &ServiceSession{}

	// 4. Occupied again -> should get dev-code-3
	id4, _, err := r.ReserveHandle("dev-code", "svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id4 != "dev-code-3" {
		t.Fatalf("expected dev-code-3, got %s", id4)
	}

	// 5. Length constraint (max 32 characters)
	longID := strings.Repeat("a", 35)
	id5, _, err := r.ReserveHandle(longID, "svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id5) > protocol.MaxIDLength {
		t.Fatalf("id length %d exceeds max %d", len(id5), protocol.MaxIDLength)
	}
}

func TestRegistry_ResolveHandle_NameSuffix(t *testing.T) {
	r := NewRegistry()

	// First allocation: name "foo" should be granted as-is.
	id1, name1, err := r.ReserveHandle("owner-a", "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1 != "owner-a" || name1 != "foo" {
		t.Fatalf("expected owner-a/foo, got %s/%s", id1, name1)
	}

	// Simulate the service being registered so byName is populated.
	r.byName["foo"] = "owner-a"
	r.services["owner-a"] = &ServiceSession{}

	// Second allocation: same name "foo" with different ID -> name should be auto-suffixed.
	id2, name2, err := r.ReserveHandle("owner-b", "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id2 != "owner-b" {
		t.Fatalf("expected id owner-b, got %s", id2)
	}
	if name2 != "foo-2" {
		t.Fatalf("expected name foo-2, got %s", name2)
	}

	// Third allocation: "foo" and "foo-2" both taken -> should get "foo-3".
	r.byName["foo-2"] = "owner-b"
	r.services["owner-b"] = &ServiceSession{}

	_, name3, err := r.ReserveHandle("owner-c", "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name3 != "foo-3" {
		t.Fatalf("expected name foo-3, got %s", name3)
	}
}

// TestRegistry_HandleTaken_CrossNamespace verifies that a new agent cannot claim an ID
// that matches an existing agent's Name, which would shadow it in Get().
func TestRegistry_HandleTaken_CrossNamespace(t *testing.T) {
	r := NewRegistry()

	// Agent A: ID="zzz", Name="yyy"
	idA, nameA, err := r.ReserveHandle("zzz", "yyy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := r.Register(protocol.ServiceInfo{ID: idA, Name: nameA}, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Agent B requests ID="yyy" — collides with A's Name in Get() lookup.
	// handleTaken must detect the byName collision and suffix the ID.
	idB, _, err := r.ReserveHandle("yyy", "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idB == "yyy" {
		t.Fatal("agent B was assigned ID \"yyy\" which shadows agent A's Name — cross-namespace hijack not prevented")
	}

	// "yyy" must still resolve to agent A.
	got, ok := r.Get("yyy")
	if !ok {
		t.Fatal("Get(\"yyy\") returned nothing — agent A was orphaned")
	}
	if got.Info.ID != idA {
		t.Fatalf("Get(\"yyy\") returned %q, want %q — routing was hijacked", got.Info.ID, idA)
	}
}

// TestRegistry_ReserveHandle_Concurrent verifies that concurrent ReserveHandle
// calls with the same preferred Name never produce duplicate IDs or Names.
func TestRegistry_ReserveHandle_Concurrent(t *testing.T) {
	const n = 20
	r := NewRegistry()

	type result struct{ id, name string }
	results := make([]result, n)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id, name, err := r.ReserveHandle("", "myapp")
			if err != nil {
				t.Errorf("goroutine %d: unexpected error: %v", i, err)
				return
			}
			results[i] = result{id, name}
		}(i)
	}
	wg.Wait()

	seenIDs := make(map[string]int)
	seenNames := make(map[string]int)
	for i, res := range results {
		if res.id == "" {
			continue // goroutine errored
		}
		if prev, dup := seenIDs[res.id]; dup {
			t.Errorf("duplicate ID %q assigned to goroutines %d and %d", res.id, prev, i)
		}
		seenIDs[res.id] = i
		if prev, dup := seenNames[res.name]; dup {
			t.Errorf("duplicate Name %q assigned to goroutines %d and %d", res.name, prev, i)
		}
		seenNames[res.name] = i
	}
}

func TestRegistryListSortedByRegistration(t *testing.T) {
	r := NewRegistry()

	// Register out of order: newest first.
	infoNew := protocol.ServiceInfo{ID: "newest", Name: "newest", ConnectedAt: time.Now().Add(2 * time.Second)}
	infoOld := protocol.ServiceInfo{ID: "oldest", Name: "oldest", ConnectedAt: time.Now()}

	if _, _, err := r.ReserveHandle("newest", "newest"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := r.Register(infoNew, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, _, err := r.ReserveHandle("oldest", "oldest"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := r.Register(infoOld, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 services, got %d", len(list))
	}
	if list[0].ID != "oldest" || list[1].ID != "newest" {
		t.Errorf("expected oldest-first order, got [%s, %s]", list[0].ID, list[1].ID)
	}
}
