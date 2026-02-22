package mapcache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/omniaura/go-kit/mapcache"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		opts    []mapcache.OptFunc
		wantErr bool
	}{
		{
			name:    "default options",
			opts:    nil,
			wantErr: false,
		},
		{
			name: "with valid size",
			opts: []mapcache.OptFunc{
				mapcache.WithSize(10),
			},
			wantErr: false,
		},
		{
			name: "with invalid size",
			opts: []mapcache.OptFunc{
				mapcache.WithSize(-1),
			},
			wantErr: true,
		},
		{
			name: "with valid TTL",
			opts: []mapcache.OptFunc{
				mapcache.WithTTL(time.Second),
			},
			wantErr: false,
		},
		{
			name: "with invalid TTL",
			opts: []mapcache.OptFunc{
				mapcache.WithTTL(-1),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mapcache.New[string, int](tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMapCache_Get(t *testing.T) {
	t.Run("basic get and cache", func(t *testing.T) {
		mc, err := mapcache.New[string, int]()
		if err != nil {
			t.Fatal(err)
		}

		calls := 0
		updater := func() (int, error) {
			calls++
			return 42, nil
		}

		// First call should invoke updater
		val, err := mc.Get("test", updater)
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
		val, err = mc.Get("test", updater)
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
		mc, err := mapcache.New[string, int](mapcache.WithTTL(50 * time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}

		calls := 0
		updater := func() (int, error) {
			calls++
			return calls, nil
		}

		// First call
		val, err := mc.Get("test", updater)
		if err != nil {
			t.Fatal(err)
		}
		if val != 1 {
			t.Errorf("expected 1, got %d", val)
		}

		// Wait for TTL to expire
		time.Sleep(100 * time.Millisecond)

		// Should get new value after TTL
		val, err = mc.Get("test", updater)
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

	t.Run("updater error", func(t *testing.T) {
		mc, err := mapcache.New[string, int]()
		if err != nil {
			t.Fatal(err)
		}

		expectedErr := errors.New("update failed")
		updater := func() (int, error) {
			return 0, expectedErr
		}

		_, err = mc.Get("test", updater)
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

func TestMapCache_Cleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mc, err := mapcache.New[string, int](
		mapcache.WithTTL(100*time.Millisecond),
		mapcache.WithCleanup(ctx, 100*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}

	updater := func() (int, error) {
		return 42, nil
	}

	// Add item
	_, err = mc.Get("test", updater)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	// Verify item was cleaned up
	var count int
	for k, v := range mc.All() {
		count++
		t.Logf("key: %s, value: %v", k, v)
	}
	if count != 0 {
		t.Errorf("expected 0 items after cleanup, got %d", count)
	}
}

func TestMapCache_All(t *testing.T) {
	mc, err := mapcache.New[string, int]()
	if err != nil {
		t.Fatal(err)
	}

	// Add some items
	items := map[string]int{
		"one":   1,
		"two":   2,
		"three": 3,
	}

	for k, v := range items {
		_, err := mc.Get(k, func() (int, error) {
			return v, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Test All iterator
	count := 0
	for k, item := range mc.All() {
		count++
		expected := items[k]
		if item.V != expected {
			t.Errorf("expected value %d for key %s, got %d", expected, k, item.V)
		}
	}

	if count != len(items) {
		t.Errorf("expected %d items, got %d", len(items), count)
	}
}
