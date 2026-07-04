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
	refreshing map[K]struct{}
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

type swrOptions[K comparable, V any] struct {
	TTL            *time.Duration
	OnStale        func(K, Item[V])
	OnRefresh      func(K, Item[V])
	OnRefreshError func(K, error)
}

type SWROptFunc[K comparable, V any] func(*swrOptions[K, V]) error

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

func WithSWRTTL[K comparable, V any](ttl time.Duration) SWROptFunc[K, V] {
	return func(o *swrOptions[K, V]) error {
		if ttl < 0 {
			return fmt.Errorf("ttl less than 0: %d", ttl)
		}
		o.TTL = &ttl
		return nil
	}
}

func WithOnStale[K comparable, V any](fn func(K, Item[V])) SWROptFunc[K, V] {
	return func(o *swrOptions[K, V]) error {
		o.OnStale = fn
		return nil
	}
}

func WithOnRefresh[K comparable, V any](fn func(K, Item[V])) SWROptFunc[K, V] {
	return func(o *swrOptions[K, V]) error {
		o.OnRefresh = fn
		return nil
	}
}

func WithOnRefreshError[K comparable, V any](fn func(K, error)) SWROptFunc[K, V] {
	return func(o *swrOptions[K, V]) error {
		o.OnRefreshError = fn
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
	mc.refreshing = make(map[K]struct{})
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

// GetSWR returns the cached value immediately when it is stale and refreshes
// that key in the background. Missing keys still block on up so callers receive
// an initial value or error.
func (mc *MapCache[K, V]) GetSWR(
	ctx context.Context,
	key K,
	up func(context.Context) (V, error),
	opts ...SWROptFunc[K, V],
) (V, error) {
	var o swrOptions[K, V]
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
		newVal, err := up(ctx)
		if err != nil {
			return newVal, err
		}
		mc.set(key, newVal, now)
		return newVal, nil
	}

	ttl := mc.TTL
	if o.TTL != nil {
		ttl = *o.TTL
	}
	if !isStale(now, item, ttl) {
		return item.V, nil
	}

	if o.OnStale != nil {
		o.OnStale(key, item)
	}
	mc.refresh(ctx, key, up, o)
	return item.V, nil
}

// At returns the currently cached item without invoking an updater or checking
// staleness.
func (mc *MapCache[K, V]) At(key K) (Item[V], bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	item, ok := mc.m[key]
	return item, ok
}

// Peek returns the currently cached item when present and not stale. Unlike Get,
// it never invokes an updater.
func (mc *MapCache[K, V]) Peek(key K, opts ...OptFunc) (Item[V], bool) {
	var o options
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return Item[V]{}, false
		}
	}
	item, ok := mc.At(key)
	if !ok {
		return Item[V]{}, false
	}
	ttl := mc.TTL
	if o.TTL != nil {
		ttl = *o.TTL
	}
	if isStale(time.Now(), item, ttl) {
		return Item[V]{}, false
	}
	return item, true
}

// Set writes value for key and resets the item's freshness timestamp.
func (mc *MapCache[K, V]) Set(key K, value V) Item[V] {
	return mc.set(key, value, time.Now())
}

func (mc *MapCache[K, V]) refresh(
	ctx context.Context,
	key K,
	up func(context.Context) (V, error),
	o swrOptions[K, V],
) {
	mc.mu.Lock()
	if mc.refreshing == nil {
		mc.refreshing = make(map[K]struct{})
	}
	if _, ok := mc.refreshing[key]; ok {
		mc.mu.Unlock()
		return
	}
	mc.refreshing[key] = struct{}{}
	mc.mu.Unlock()

	go func() {
		defer func() {
			mc.mu.Lock()
			delete(mc.refreshing, key)
			mc.mu.Unlock()
		}()

		newVal, err := up(ctx)
		if err != nil {
			if o.OnRefreshError != nil {
				o.OnRefreshError(key, err)
			}
			return
		}
		item := mc.set(key, newVal, time.Now())
		if o.OnRefresh != nil {
			o.OnRefresh(key, item)
		}
	}()
}

func (mc *MapCache[K, V]) set(key K, value V, now time.Time) Item[V] {
	item := Item[V]{
		V:         value,
		UpdatedAt: now,
	}
	mc.mu.Lock()
	mc.m[key] = item
	mc.mu.Unlock()
	return item
}

func isStale[V any](now time.Time, item Item[V], ttl time.Duration) bool {
	return ttl > 0 && now.Sub(item.UpdatedAt) >= ttl
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
