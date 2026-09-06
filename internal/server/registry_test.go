package server

import (
	"strings"
	"testing"

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
