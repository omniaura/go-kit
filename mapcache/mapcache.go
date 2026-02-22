package mapcache

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"time"
)

type MapCache[K comparable, V any] struct {
	m          map[K]Item[V]
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
	Size            *int
	CleanupInterval *time.Duration
	CleanupCtx      context.Context
}

type OptFunc func(*options) error

func WithSize(size int) OptFunc {
	return func(o *options) error {
		if size < 0 {
			return fmt.Errorf("size less than 0: %d", size)
		}
		o.Size = &size
		return nil
	}
}

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

func New[K comparable, V any](opts ...OptFunc) (*MapCache[K, V], error) {
	var o options
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return nil, err
		}
	}
	var mc MapCache[K, V]
	if o.Size != nil {
		mc.m = make(map[K]Item[V], *o.Size)
	} else {
		mc.m = make(map[K]Item[V])
	}
	if o.TTL != nil {
		mc.TTL = *o.TTL
	}
	if o.CleanupInterval != nil {
		if err := mc.cleanupRoutine(o.CleanupCtx, *o.CleanupInterval); err != nil {
			return nil, err
		}

	}
	return &mc, nil
}

func (mc *MapCache[K, V]) cleanupRoutine(ctx context.Context, interval time.Duration) error {
	if mc.TTL == 0 {
		return errors.New("WithCleanup option is not valid for TTL 0 (value lives forever)")
	}
	if mc.TTL < 0 {
		return errors.New("withCleanup option is not valid for TTL less than 0")
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(interval):
				now := time.Now()
				mc.mu.Lock()
				for k, v := range mc.m {
					if now.Sub(v.UpdatedAt) > mc.TTL {
						delete(mc.m, k)
					}
				}
				mc.mu.Unlock()
			}
		}
	}()
	return nil
}

func (mc *MapCache[K, V]) Get(key K, up func() (V, error), opts ...OptFunc) (V, error) {
	var o options
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			var v V
			return v, err
		}
	}

	mc.mu.RLock()
	item, ok := mc.m[key]
	mc.mu.RUnlock()
	now := time.Now()
	if !ok {
		newVal, err := up()
		if err != nil {
			return newVal, err
		}
		mc.mu.Lock()
		mc.m[key] = Item[V]{
			V:         newVal,
			UpdatedAt: now,
		}
		mc.mu.Unlock()
		return newVal, nil
	}
	ttl := mc.TTL
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
	newVal, err := up()
	if err != nil {
		return newVal, err
	}
	mc.mu.Lock()
	mc.m[key] = Item[V]{
		V:         newVal,
		UpdatedAt: now,
	}
	mc.mu.Unlock()
	return newVal, nil
}

func (mc *MapCache[K, V]) AllParallel() iter.Seq2[K, Item[V]] {
	return func(yield func(K, Item[V]) bool) {
		mc.mu.RLock()
		defer mc.mu.RUnlock()
		for k, v := range mc.m {
			go func() {
				yield(k, v)
			}()
		}
	}
}

func (mc *MapCache[K, V]) All() iter.Seq2[K, Item[V]] {
	return func(yield func(K, Item[V]) bool) {
		mc.mu.RLock()
		defer mc.mu.RUnlock()
		for k, v := range mc.m {
			if !yield(k, v) {
				return
			}
		}
	}
}
