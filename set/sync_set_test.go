package set_test

import (
	"slices"
	"sync"
	"testing"

	"github.com/omniaura/go-kit/set"
)

func TestSyncSet_ClaimAddsMissingKey(t *testing.T) {
	t.Run("adds missing key", func(t *testing.T) {
		s := set.NewSync[string]()

		if got := s.Claim("a"); !got {
			t.Fatalf("Claim(%q) = %v, want true", "a", got)
		}
		if got := s.Len(); got != 1 {
			t.Fatalf("Len() = %d, want 1", got)
		}
		if !s.Contains("a") {
			t.Fatalf("Contains(%q) = false, want true", "a")
		}
	})

	t.Run("does not add existing key", func(t *testing.T) {
		s := set.NewSync[string]()
		s.Add("a")

		if got := s.Claim("a"); got {
			t.Fatalf("Claim(%q) = %v, want false", "a", got)
		}
		if got := s.Len(); got != 1 {
			t.Fatalf("Len() = %d, want 1", got)
		}
	})
}

func TestSyncSet_Claim(t *testing.T) {
	s := set.NewSync[string]()

	if got := s.Claim("job-1"); !got {
		t.Fatalf("Claim(%q) = %v, want true", "job-1", got)
	}
	if got := s.Claim("job-1"); got {
		t.Fatalf("Claim(%q) = %v, want false", "job-1", got)
	}
	if got := s.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
}

func TestSyncSet_AddContains(t *testing.T) {
	tests := []struct {
		name     string
		add      []string
		check    string
		contains bool
	}{
		{
			name:     "empty set does not contain key",
			add:      nil,
			check:    "a",
			contains: false,
		},
		{
			name:     "set contains added key",
			add:      []string{"a"},
			check:    "a",
			contains: true,
		},
		{
			name:     "set does not contain missing key",
			add:      []string{"a", "b"},
			check:    "c",
			contains: false,
		},
		{
			name:     "duplicate adds are idempotent",
			add:      []string{"a", "a", "a"},
			check:    "a",
			contains: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := set.NewSync[string]()
			for _, k := range tt.add {
				s.Add(k)
			}
			if got := s.Contains(tt.check); got != tt.contains {
				t.Errorf("Contains(%q) = %v, want %v", tt.check, got, tt.contains)
			}
			if got := s.Missing(tt.check); got != !tt.contains {
				t.Errorf("Missing(%q) = %v, want %v", tt.check, got, !tt.contains)
			}
		})
	}
}

func TestSyncSet_Remove(t *testing.T) {
	tests := []struct {
		name         string
		add          []string
		remove       []string
		checkContain string
		contains     bool
		wantLen      int
	}{
		{
			name:         "remove from empty set",
			add:          nil,
			remove:       []string{"a"},
			checkContain: "a",
			contains:     false,
			wantLen:      0,
		},
		{
			name:         "remove existing key",
			add:          []string{"a", "b"},
			remove:       []string{"a"},
			checkContain: "a",
			contains:     false,
			wantLen:      1,
		},
		{
			name:         "remove non-existing key",
			add:          []string{"a", "b"},
			remove:       []string{"c"},
			checkContain: "a",
			contains:     true,
			wantLen:      2,
		},
		{
			name:         "remove all keys",
			add:          []string{"a", "b"},
			remove:       []string{"a", "b"},
			checkContain: "a",
			contains:     false,
			wantLen:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := set.NewSync[string]()
			for _, k := range tt.add {
				s.Add(k)
			}
			for _, k := range tt.remove {
				s.Remove(k)
			}
			if got := s.Contains(tt.checkContain); got != tt.contains {
				t.Errorf("Contains(%q) = %v, want %v", tt.checkContain, got, tt.contains)
			}
			if got := s.Len(); got != tt.wantLen {
				t.Errorf("Len() = %v, want %v", got, tt.wantLen)
			}
		})
	}
}

func TestSyncSet_Clear(t *testing.T) {
	tests := []struct {
		name    string
		add     []string
		wantLen int
	}{
		{
			name:    "clear empty set",
			add:     nil,
			wantLen: 0,
		},
		{
			name:    "clear set with one element",
			add:     []string{"a"},
			wantLen: 0,
		},
		{
			name:    "clear set with multiple elements",
			add:     []string{"a", "b", "c"},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := set.NewSync[string]()
			for _, k := range tt.add {
				s.Add(k)
			}
			s.Clear()
			if got := s.Len(); got != tt.wantLen {
				t.Errorf("Len() after Clear() = %v, want %v", got, tt.wantLen)
			}
		})
	}
}

func TestSyncSet_Len(t *testing.T) {
	tests := []struct {
		name    string
		add     []string
		wantLen int
	}{
		{
			name:    "empty set",
			add:     nil,
			wantLen: 0,
		},
		{
			name:    "one element",
			add:     []string{"a"},
			wantLen: 1,
		},
		{
			name:    "multiple elements",
			add:     []string{"a", "b", "c"},
			wantLen: 3,
		},
		{
			name:    "duplicates do not increase length",
			add:     []string{"a", "a", "b", "b", "b"},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := set.NewSync[string]()
			for _, k := range tt.add {
				s.Add(k)
			}
			if got := s.Len(); got != tt.wantLen {
				t.Errorf("Len() = %v, want %v", got, tt.wantLen)
			}
		})
	}
}

func TestSyncSet_Iter(t *testing.T) {
	tests := []struct {
		name string
		add  []string
		want []string
	}{
		{
			name: "empty set",
			add:  nil,
			want: nil,
		},
		{
			name: "one element",
			add:  []string{"a"},
			want: []string{"a"},
		},
		{
			name: "multiple elements",
			add:  []string{"a", "b", "c"},
			want: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := set.NewSync[string]()
			for _, k := range tt.add {
				s.Add(k)
			}
			var got []string
			for k := range s.Iter() {
				got = append(got, k)
			}
			slices.Sort(got)
			slices.Sort(tt.want)
			if len(got) != len(tt.want) {
				t.Errorf("Iter() returned %d elements, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Iter() element %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSyncSet_Concurrent(t *testing.T) {
	s := set.NewSync[int]()
	const numGoroutines = 100
	const numOps = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3)

	// Concurrent adds
	for i := range numGoroutines {
		go func(base int) {
			defer wg.Done()
			for j := range numOps {
				s.Add(base*numOps + j)
			}
		}(i)
	}

	// Concurrent reads
	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range numOps {
				s.Contains(0)
				s.Missing(0)
				s.Len()
			}
		}()
	}

	// Concurrent removes
	for i := range numGoroutines {
		go func(base int) {
			defer wg.Done()
			for j := range numOps {
				s.Remove(base*numOps + j)
			}
		}(i)
	}

	wg.Wait()

	// Should complete without race conditions
	// Final state is non-deterministic due to concurrent add/remove
}

func TestSyncSet_ClaimConcurrent(t *testing.T) {
	s := set.NewSync[string]()
	const numGoroutines = 128

	start := make(chan struct{})
	results := make(chan bool, numGoroutines)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for range numGoroutines {
		go func() {
			defer wg.Done()
			<-start
			results <- s.Claim("request-1")
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	var won int
	for result := range results {
		if result {
			won++
		}
	}

	if won != 1 {
		t.Fatalf("Claim() winners = %d, want 1", won)
	}
	if !s.Contains("request-1") {
		t.Fatalf("Contains(%q) = false, want true", "request-1")
	}
	if got := s.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
}
