package server

import (
	"strings"
	"testing"
	"time"

	"github.com/gosuda/maek/internal/protocol"
)

func TestRegistry_AllocateID(t *testing.T) {
	r := NewRegistry()

	// 1. Empty preferred -> 6-char random
	id1, err := r.AllocateID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id1) != protocol.IDLength {
		t.Fatalf("expected length %d, got %d (id: %s)", protocol.IDLength, len(id1), id1)
	}

	// 2. Unoccupied preferred ID
	id2, err := r.AllocateID("dev-code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id2 != "dev-code" {
		t.Fatalf("expected dev-code, got %s", id2)
	}

	// Mock registration of id2 so it becomes occupied
	r.services["dev-code"] = &ServiceSession{}

	// 3. Occupied preferred ID -> should get dev-code-2
	id3, err := r.AllocateID("dev-code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id3 != "dev-code-2" {
		t.Fatalf("expected dev-code-2, got %s", id3)
	}

	// Mock registration of dev-code-2
	r.services["dev-code-2"] = &ServiceSession{}

	// 4. Occupied again -> should get dev-code-3
	id4, err := r.AllocateID("dev-code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id4 != "dev-code-3" {
		t.Fatalf("expected dev-code-3, got %s", id4)
	}

	// 5. Length constraint (max 32 characters)
	longID := strings.Repeat("a", 35)
	id5, err := r.AllocateID(longID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id5) > protocol.MaxIDLength {
		t.Fatalf("id length %d exceeds max %d", len(id5), protocol.MaxIDLength)
	}
}

func TestRegistryListSortedByRegistration(t *testing.T) {
	r := NewRegistry()

	// Register out of order: newest first.
	infoNew := protocol.ServiceInfo{ID: "newest", Name: "newest", ConnectedAt: time.Now().Add(2 * time.Second)}
	infoOld := protocol.ServiceInfo{ID: "oldest", Name: "oldest", ConnectedAt: time.Now()}

	if _, err := r.Register(infoNew, nil, nil); err != nil {
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
