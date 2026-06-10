package batcher

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// sink collects flushed batches for assertions.
type sink struct {
	mu      sync.Mutex
	batches [][]int
	err     error
}

func (s *sink) flush(_ context.Context, batch []int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batches = append(s.batches, batch)
	return s.err
}

func (s *sink) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, b := range s.batches {
		n += len(b)
	}
	return n
}

func (s *sink) batchCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.batches)
}

func TestFlushOnMaxBatch(t *testing.T) {
	var s sink
	b, err := New(s.flush, WithMaxBatch(3), WithInterval(time.Hour))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close(context.Background())

	for i := range 6 {
		if !b.Add(i) {
			t.Fatalf("Add(%d) dropped", i)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for s.batchCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := s.batchCount(); got != 2 {
		t.Fatalf("batch count = %d, want 2", got)
	}
	if got := s.total(); got != 6 {
		t.Fatalf("flushed items = %d, want 6", got)
	}
}

func TestFlushOnInterval(t *testing.T) {
	var s sink
	b, err := New(s.flush, WithMaxBatch(100), WithInterval(20*time.Millisecond))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close(context.Background())

	b.Add(1)
	b.Add(2)

	deadline := time.Now().Add(2 * time.Second)
	for s.total() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := s.total(); got != 2 {
		t.Fatalf("flushed items = %d, want 2 (interval flush)", got)
	}
}

func TestDropWhenFull(t *testing.T) {
	block := make(chan struct{})
	var entered sync.Once
	flushEntered := make(chan struct{})
	flush := func(_ context.Context, _ []int) error {
		entered.Do(func() { close(flushEntered) })
		<-block
		return nil
	}
	b, err := New(flush, WithCapacity(2), WithMaxBatch(1), WithInterval(time.Hour))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// First item is taken by the worker, which then blocks in flush.
	b.Add(0)
	<-flushEntered
	// Fill the channel buffer, then overflow it.
	b.Add(1)
	b.Add(2)
	accepted := b.Add(3)
	dropped := b.Dropped()

	close(block)
	b.Close(context.Background())

	if accepted {
		t.Fatal("Add succeeded on a full buffer, want drop")
	}
	if dropped == 0 {
		t.Fatalf("Dropped() = 0, want > 0")
	}
}

func TestCloseFlushesRemainder(t *testing.T) {
	var s sink
	b, err := New(s.flush, WithMaxBatch(100), WithInterval(time.Hour))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for i := range 5 {
		b.Add(i)
	}
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := s.total(); got != 5 {
		t.Fatalf("flushed items = %d, want 5 after Close", got)
	}
}

func TestAddAfterClose(t *testing.T) {
	var s sink
	b, err := New(s.flush)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if b.Add(1) {
		t.Fatal("Add succeeded after Close")
	}
	if got := b.Dropped(); got != 1 {
		t.Fatalf("Dropped() = %d, want 1", got)
	}
	// Idempotent close.
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestCloseTimeout(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	flushEntered := make(chan struct{})
	var entered sync.Once
	flush := func(_ context.Context, _ []int) error {
		entered.Do(func() { close(flushEntered) })
		<-block
		return nil
	}
	b, err := New(flush, WithMaxBatch(1), WithInterval(time.Hour))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b.Add(1)
	<-flushEntered

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := b.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close error = %v, want DeadlineExceeded", err)
	}
}

func TestOnError(t *testing.T) {
	var got atomic.Int64
	s := sink{err: errors.New("boom")}
	b, err := New(s.flush, WithMaxBatch(1), WithInterval(time.Hour), WithOnError(func(error) { got.Add(1) }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b.Add(1)
	b.Close(context.Background())
	if got.Load() != 1 {
		t.Fatalf("onError calls = %d, want 1", got.Load())
	}
}

func TestConcurrentAddAndClose(t *testing.T) {
	var s sink
	b, err := New(s.flush, WithCapacity(64), WithMaxBatch(8), WithInterval(time.Millisecond))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for i := range 1000 {
				b.Add(i)
			}
		})
	}
	wg.Wait()
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	delivered := uint64(s.total())
	if delivered+b.Dropped() != 8000 {
		t.Fatalf("delivered %d + dropped %d != 8000", delivered, b.Dropped())
	}
}

func TestOptionValidation(t *testing.T) {
	flush := func(context.Context, []int) error { return nil }
	if _, err := New[int](nil); err == nil {
		t.Fatal("New(nil flush) should error")
	}
	if _, err := New(flush, WithCapacity(0)); err == nil {
		t.Fatal("WithCapacity(0) should error")
	}
	if _, err := New(flush, WithMaxBatch(-1)); err == nil {
		t.Fatal("WithMaxBatch(-1) should error")
	}
	if _, err := New(flush, WithInterval(0)); err == nil {
		t.Fatal("WithInterval(0) should error")
	}
	if _, err := New(flush, WithFlushTimeout(0)); err == nil {
		t.Fatal("WithFlushTimeout(0) should error")
	}
	if _, err := New(flush, WithOnError(nil)); err == nil {
		t.Fatal("WithOnError(nil) should error")
	}
}
