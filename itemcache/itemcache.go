package itemcache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ItemCache caches a single value, refreshing it via an updater function
// once the configured TTL has elapsed.
type ItemCache[V any] struct {
	item       Item[V]
	set        bool
	mu         sync.RWMutex
	TTL        time.Duration
	cleanupCtx context.Context
}

type Item[V any] struct {
	V         V
	UpdatedAt time.Time
}

type options struct {
	TTL             *time.Duration
	CleanupInterval *time.Duration
	CleanupCtx      context.Context
}

type OptFunc func(*options) error

func WithTTL(ttl time.Duration) OptFunc {
	return func(o *options) error {
		if ttl < 0 {
			return fmt.Errorf("ttl less than 0: %d", ttl)
		}
		o.TTL = &ttl
		return nil
	}
}

func WithCleanup(ctx context.Context, interval time.Duration) OptFunc {
	return func(o *options) error {
		if interval < 0 {
			return fmt.Errorf("interval less than 0: %d", interval)
		}
		o.CleanupCtx = ctx
		o.CleanupInterval = &interval
		return nil
	}
}

func New[V any](opts ...OptFunc) (*ItemCache[V], error) {
	var o options
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return nil, err
		}
	}
	var ic ItemCache[V]
	if o.TTL != nil {
		ic.TTL = *o.TTL
	}
	if o.CleanupInterval != nil {
		if err := ic.cleanupRoutine(o.CleanupCtx, *o.CleanupInterval); err != nil {
			return nil, err
		}
	}
	return &ic, nil
}

func (ic *ItemCache[V]) cleanupRoutine(ctx context.Context, interval time.Duration) error {
	if ic.TTL == 0 {
		return errors.New("WithCleanup option is not valid for TTL 0 (value lives forever)")
	}
	if ic.TTL < 0 {
		return errors.New("withCleanup option is not valid for TTL less than 0")
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(interval):
				now := time.Now()
				ic.mu.Lock()
				if ic.set && now.Sub(ic.item.UpdatedAt) > ic.TTL {
					ic.set = false
					var zero Item[V]
					ic.item = zero
				}
				ic.mu.Unlock()
			}
		}
	}()
	return nil
}

// Get returns the cached value, calling up to populate or refresh it when the
// value is missing or has exceeded the TTL.
func (ic *ItemCache[V]) Get(up func() (V, error), opts ...OptFunc) (V, error) {
	var o options
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			var v V
			return v, err
		}
	}

	ic.mu.RLock()
	item, ok := ic.item, ic.set
	ic.mu.RUnlock()
	now := time.Now()
	if !ok {
		return ic.update(up, now)
	}
	ttl := ic.TTL
	if o.TTL != nil {
		ttl = *o.TTL
	}

	if ttl == 0 {
		return item.V, nil
	}
	age := now.Sub(item.UpdatedAt)
	if age < ttl {
		return item.V, nil
	}
	return ic.update(up, now)
}

func (ic *ItemCache[V]) update(up func() (V, error), now time.Time) (V, error) {
	newVal, err := up()
	if err != nil {
		return newVal, err
	}
	ic.mu.Lock()
	ic.item = Item[V]{
		V:         newVal,
		UpdatedAt: now,
	}
	ic.set = true
	ic.mu.Unlock()
	return newVal, nil
}

// Peek returns the currently cached item without triggering an update. The
// boolean reports whether a value is currently cached.
func (ic *ItemCache[V]) Peek() (Item[V], bool) {
	ic.mu.RLock()
	defer ic.mu.RUnlock()
	return ic.item, ic.set
}

// Clear removes the cached value, forcing the next Get to call its updater.
func (ic *ItemCache[V]) Clear() {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	var zero Item[V]
	ic.item = zero
	ic.set = false
}
