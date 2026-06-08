package itemcache_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/omniaura/go-kit/itemcache"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		opts    []itemcache.OptFunc
		wantErr bool
	}{
		{
			name:    "default options",
			opts:    nil,
			wantErr: false,
		},
		{
			name: "with valid TTL",
			opts: []itemcache.OptFunc{
				itemcache.WithTTL(time.Second),
			},
			wantErr: false,
		},
		{
			name: "with invalid TTL",
			opts: []itemcache.OptFunc{
				itemcache.WithTTL(-1),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := itemcache.New[int](tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestItemCache_Get(t *testing.T) {
	t.Run("basic get and cache", func(t *testing.T) {
		ic, err := itemcache.New[int]()
		if err != nil {
			t.Fatal(err)
		}

		calls := 0
		updater := func() (int, error) {
			calls++
			return 42, nil
		}

		// First call should invoke updater
		val, err := ic.Get(updater)
		if err != nil {
			t.Fatal(err)
		}
		if val != 42 {
			t.Errorf("expected 42, got %d", val)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}

		// Second call should use cached value
		val, err = ic.Get(updater)
		if err != nil {
			t.Fatal(err)
		}
		if val != 42 {
			t.Errorf("expected 42, got %d", val)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("with TTL", func(t *testing.T) {
		ic, err := itemcache.New[int](itemcache.WithTTL(50 * time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}

		calls := 0
		updater := func() (int, error) {
			calls++
			return calls, nil
		}

		// First call
		val, err := ic.Get(updater)
		if err != nil {
			t.Fatal(err)
		}
		if val != 1 {
			t.Errorf("expected 1, got %d", val)
		}

		// Wait for TTL to expire
		time.Sleep(100 * time.Millisecond)

		// Should get new value after TTL
		val, err = ic.Get(updater)
		if err != nil {
			t.Fatal(err)
		}
		if val != 2 {
			t.Errorf("expected 2, got %d", val)
		}
		if calls != 2 {
			t.Errorf("expected 2 calls, got %d", calls)
		}
	})

	t.Run("per-call TTL override", func(t *testing.T) {
		ic, err := itemcache.New[int]()
		if err != nil {
			t.Fatal(err)
		}

		calls := 0
		updater := func() (int, error) {
			calls++
			return calls, nil
		}

		if _, err := ic.Get(updater); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)

		// TTL override forces a refresh since the value is older than 10ms.
		val, err := ic.Get(updater, itemcache.WithTTL(10*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		if val != 2 {
			t.Errorf("expected 2, got %d", val)
		}
	})

	t.Run("updater error", func(t *testing.T) {
		ic, err := itemcache.New[int]()
		if err != nil {
			t.Fatal(err)
		}

		expectedErr := errors.New("update failed")
		updater := func() (int, error) {
			return 0, expectedErr
		}

		_, err = ic.Get(updater)
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

// TestItemCache_Concurrent exercises the cache from many goroutines while a
// cleanup routine runs, relying on the race detector (go test -race) to catch
// unsynchronized access to the shared item.
func TestItemCache_Concurrent(t *testing.T) {
	ctx := t.Context()

	ic, err := itemcache.New[int](
		itemcache.WithTTL(time.Millisecond),
		itemcache.WithCleanup(ctx, time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int64
	updater := func() (int, error) {
		return int(calls.Add(1)), nil
	}

	const goroutines = 50
	const iterations = 200

	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := range iterations {
				switch (g + i) % 3 {
				case 0:
					if _, err := ic.Get(updater); err != nil {
						t.Errorf("Get: %v", err)
						return
					}
				case 1:
					ic.Peek()
				case 2:
					ic.Clear()
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestItemCache_Cleanup(t *testing.T) {
	ctx := t.Context()

	ic, err := itemcache.New[int](
		itemcache.WithTTL(100*time.Millisecond),
		itemcache.WithCleanup(ctx, 100*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}

	updater := func() (int, error) {
		return 42, nil
	}

	// Populate the cache
	if _, err = ic.Get(updater); err != nil {
		t.Fatal(err)
	}

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	// Verify item was cleaned up
	if _, ok := ic.Peek(); ok {
		t.Errorf("expected no cached item after cleanup")
	}
}

func TestItemCache_Peek(t *testing.T) {
	ic, err := itemcache.New[int]()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := ic.Peek(); ok {
		t.Errorf("expected no cached item before any Get")
	}

	if _, err := ic.Get(func() (int, error) { return 7, nil }); err != nil {
		t.Fatal(err)
	}

	item, ok := ic.Peek()
	if !ok {
		t.Fatal("expected a cached item after Get")
	}
	if item.V != 7 {
		t.Errorf("expected 7, got %d", item.V)
	}
}

func TestItemCache_Clear(t *testing.T) {
	ic, err := itemcache.New[int]()
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	updater := func() (int, error) {
		calls++
		return calls, nil
	}

	if _, err := ic.Get(updater); err != nil {
		t.Fatal(err)
	}
	ic.Clear()
	if _, ok := ic.Peek(); ok {
		t.Errorf("expected no cached item after Clear")
	}

	// Next Get should re-invoke the updater.
	val, err := ic.Get(updater)
	if err != nil {
		t.Fatal(err)
	}
	if val != 2 {
		t.Errorf("expected 2, got %d", val)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}
