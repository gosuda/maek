package protocol

import (
	"regexp"
	"testing"
)

func TestGenerateID(t *testing.T) {
	seen := make(map[string]bool)
	idRegex := regexp.MustCompile(`^[a-z0-9]{6}$`)

	for i := 0; i < 1000; i++ {
		id, err := GenerateID()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(id) != IDLength {
			t.Fatalf("expected length %d, got %d (id: %s)", IDLength, len(id), id)
		}
		if !idRegex.MatchString(id) {
			t.Fatalf("id %s does not match expected pattern", id)
		}
		if seen[id] {
			t.Fatalf("collision detected for id %s after %d iterations", id, i)
		}
		seen[id] = true
	}
}
