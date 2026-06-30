package set_test

import (
	"slices"
	"testing"

	"github.com/omniaura/go-kit/set"
)

func TestNewWithKeys(t *testing.T) {
	s := set.New[string](set.WithKeys([]string{"b", "a", "b"}))

	if got := len(s); got != 2 {
		t.Fatalf("len(New(WithKeys())) = %d, want 2", got)
	}
	for _, key := range []string{"a", "b"} {
		if !s.Contains(key) {
			t.Fatalf("Contains(%q) = false, want true", key)
		}
	}
}

func TestNewWithKeysAndCapacity(t *testing.T) {
	s := set.New[int](
		set.WithCapacity(8),
		set.WithKeys([]int{1, 2, 3}),
	)

	if got := len(s); got != 3 {
		t.Fatalf("len(New(WithCapacity(), WithKeys())) = %d, want 3", got)
	}
	if !s.Contains(1) || !s.Contains(2) || !s.Contains(3) {
		t.Fatalf("New(WithKeys()) did not seed all keys")
	}
}

func TestNewSyncWithKeys(t *testing.T) {
	s := set.NewSync[string](set.WithKeys([]string{"c", "a", "b"}))

	got := s.Slice()
	slices.Sort(got)
	want := []string{"a", "b", "c"}

	if !slices.Equal(got, want) {
		t.Fatalf("Slice() = %v, want %v", got, want)
	}
}
