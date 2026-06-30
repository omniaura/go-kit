package mapcache_test

import (
	"context"
	"errors"
	"sync/atomic"
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

func TestMapCache_At(t *testing.T) {
	mc, err := mapcache.New[string, int]()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := mc.At("missing"); ok {
		t.Fatal("expected missing key")
	}

	if _, err := mc.Get("key", func() (int, error) { return 42, nil }); err != nil {
		t.Fatal(err)
	}

	item, ok := mc.At("key")
	if !ok {
		t.Fatal("expected cached item")
	}
	if item.V != 42 {
		t.Fatalf("expected 42, got %d", item.V)
	}
}

func TestMapCache_GetSWRReturnsStaleAndRefreshes(t *testing.T) {
	mc, err := mapcache.New[string, int](mapcache.WithTTL(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := mc.Get("key", func() (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)

	started := make(chan struct{})
	release := make(chan struct{})
	stale := make(chan mapcache.Item[int], 1)
	refreshed := make(chan mapcache.Item[int], 1)
	var calls atomic.Int64

	val, err := mc.GetSWR(
		context.Background(),
		"key",
		func(ctx context.Context) (int, error) {
			calls.Add(1)
			close(started)
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-release:
				return 2, nil
			}
		},
		mapcache.WithOnStale(func(key string, item mapcache.Item[int]) {
			if key != "key" {
				t.Errorf("unexpected stale key: %s", key)
			}
			stale <- item
		}),
		mapcache.WithOnRefresh(func(key string, item mapcache.Item[int]) {
			if key != "key" {
				t.Errorf("unexpected refresh key: %s", key)
			}
			refreshed <- item
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if val != 1 {
		t.Fatalf("expected stale value 1, got %d", val)
	}

	select {
	case item := <-stale:
		if item.V != 1 {
			t.Fatalf("expected stale hook value 1, got %d", item.V)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stale hook")
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for refresh start")
	}

	item, ok := mc.At("key")
	if !ok || item.V != 1 {
		t.Fatalf("expected stale cached value, got %+v ok=%v", item, ok)
	}

	close(release)
	select {
	case item := <-refreshed:
		if item.V != 2 {
			t.Fatalf("expected refreshed value 2, got %d", item.V)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for refresh")
	}

	item, ok = mc.At("key")
	if !ok || item.V != 2 {
		t.Fatalf("expected refreshed cached value, got %+v ok=%v", item, ok)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 refresh call, got %d", got)
	}
}

func TestMapCache_GetSWRDeduplicatesRefresh(t *testing.T) {
	mc, err := mapcache.New[string, int](mapcache.WithTTL(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := mc.Get("key", func() (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)

	started := make(chan struct{})
	release := make(chan struct{})
	refreshed := make(chan mapcache.Item[int], 1)
	var calls atomic.Int64
	up := func(ctx context.Context) (int, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-release:
			return 2, nil
		}
	}

	for range 2 {
		val, err := mc.GetSWR(
			context.Background(),
			"key",
			up,
			mapcache.WithOnRefresh(func(key string, item mapcache.Item[int]) {
				refreshed <- item
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		if val != 1 {
			t.Fatalf("expected stale value 1, got %d", val)
		}
		select {
		case <-started:
		default:
		}
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for refresh start")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected one in-flight refresh, got %d", got)
	}

	close(release)
	select {
	case item := <-refreshed:
		if item.V != 2 {
			t.Fatalf("expected refreshed value 2, got %d", item.V)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for refresh")
	}
}
