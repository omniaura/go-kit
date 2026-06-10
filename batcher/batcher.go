// Package batcher provides a bounded, non-blocking batch collector: items are
// buffered on a channel and flushed by a single background worker, either when
// the batch reaches a size threshold or on an interval. Adds never block and
// never spawn goroutines — when the buffer is full the item is dropped and
// counted — so a slow or unavailable sink cannot stall callers or leak
// goroutines under burst. Designed for fire-and-forget workloads such as
// telemetry and usage logs, where bounded resource use matters more than
// guaranteed delivery.
package batcher

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	DefaultCapacity     = 1024
	DefaultMaxBatch     = 128
	DefaultInterval     = 2 * time.Second
	DefaultFlushTimeout = 10 * time.Second
)

// FlushFunc delivers one batch to the sink. The slice is owned by the callee;
// the batcher never reuses it after the call. ctx carries the configured
// flush timeout.
type FlushFunc[T any] func(ctx context.Context, batch []T) error

// Batcher collects items and flushes them in batches via a single background
// worker started by New.
type Batcher[T any] struct {
	ch           chan T
	flush        FlushFunc[T]
	maxBatch     int
	interval     time.Duration
	flushTimeout time.Duration
	onError      func(error)

	dropped atomic.Uint64

	mu     sync.RWMutex // guards closed vs. sends on ch
	closed bool
	once   sync.Once
	done   chan struct{}
}

type options struct {
	capacity     *int
	maxBatch     *int
	interval     *time.Duration
	flushTimeout *time.Duration
	onError      func(error)
}

type OptFunc func(*options) error

// WithCapacity sets the buffered-channel capacity (default 1024). Items added
// while the buffer is full are dropped.
func WithCapacity(n int) OptFunc {
	return func(o *options) error {
		if n <= 0 {
			return fmt.Errorf("capacity must be positive: %d", n)
		}
		o.capacity = &n
		return nil
	}
}

// WithMaxBatch sets the batch size that triggers an immediate flush
// (default 128).
func WithMaxBatch(n int) OptFunc {
	return func(o *options) error {
		if n <= 0 {
			return fmt.Errorf("max batch must be positive: %d", n)
		}
		o.maxBatch = &n
		return nil
	}
}

// WithInterval sets how often a partial batch is flushed (default 2s).
func WithInterval(d time.Duration) OptFunc {
	return func(o *options) error {
		if d <= 0 {
			return fmt.Errorf("interval must be positive: %d", d)
		}
		o.interval = &d
		return nil
	}
}

// WithFlushTimeout bounds the context passed to each FlushFunc call
// (default 10s).
func WithFlushTimeout(d time.Duration) OptFunc {
	return func(o *options) error {
		if d <= 0 {
			return fmt.Errorf("flush timeout must be positive: %d", d)
		}
		o.flushTimeout = &d
		return nil
	}
}

// WithOnError registers a callback invoked with each FlushFunc error. The
// failed batch is discarded either way; without a callback errors are
// silently dropped.
func WithOnError(fn func(error)) OptFunc {
	return func(o *options) error {
		if fn == nil {
			return errors.New("onError callback must not be nil")
		}
		o.onError = fn
		return nil
	}
}

// New starts the background worker and returns the batcher. Call Close to
// flush remaining items and stop the worker.
func New[T any](flush FlushFunc[T], opts ...OptFunc) (*Batcher[T], error) {
	if flush == nil {
		return nil, errors.New("flush func must not be nil")
	}
	var o options
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return nil, err
		}
	}
	b := &Batcher[T]{
		flush:        flush,
		maxBatch:     DefaultMaxBatch,
		interval:     DefaultInterval,
		flushTimeout: DefaultFlushTimeout,
		onError:      o.onError,
		done:         make(chan struct{}),
	}
	capacity := DefaultCapacity
	if o.capacity != nil {
		capacity = *o.capacity
	}
	if o.maxBatch != nil {
		b.maxBatch = *o.maxBatch
	}
	if o.interval != nil {
		b.interval = *o.interval
	}
	if o.flushTimeout != nil {
		b.flushTimeout = *o.flushTimeout
	}
	b.ch = make(chan T, capacity)
	go b.run()
	return b, nil
}

// Add enqueues an item without blocking. It reports false when the item was
// dropped because the buffer is full or the batcher is closed.
func (b *Batcher[T]) Add(item T) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		b.dropped.Add(1)
		return false
	}
	select {
	case b.ch <- item:
		return true
	default:
		b.dropped.Add(1)
		return false
	}
}

// Dropped returns the number of items discarded because the buffer was full
// or the batcher was closed.
func (b *Batcher[T]) Dropped() uint64 {
	return b.dropped.Load()
}

// Close stops accepting items, flushes everything still buffered, and waits
// for the worker to exit or ctx to be done, whichever comes first. It is safe
// to call multiple times.
func (b *Batcher[T]) Close(ctx context.Context) error {
	b.once.Do(func() {
		b.mu.Lock()
		b.closed = true
		close(b.ch)
		b.mu.Unlock()
	})
	select {
	case <-b.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Batcher[T]) run() {
	defer close(b.done)
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	batch := make([]T, 0, b.maxBatch)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), b.flushTimeout)
		err := b.flush(ctx, batch)
		cancel()
		if err != nil && b.onError != nil {
			b.onError(err)
		}
		// Fresh slice: FlushFunc owns the one it was handed.
		batch = make([]T, 0, b.maxBatch)
	}

	for {
		select {
		case item, ok := <-b.ch:
			if !ok {
				// Closed: the channel has already been drained of buffered
				// items by this loop, so one final flush completes shutdown.
				flush()
				return
			}
			batch = append(batch, item)
			if len(batch) >= b.maxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
