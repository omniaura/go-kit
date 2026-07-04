package hit

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter blocks until one request is allowed.
type RateLimiter interface {
	Wait(context.Context) error
}

// NewLimiter returns a blocking limiter for n requests per interval.
func NewLimiter(n int, interval time.Duration) (*rate.Limiter, error) {
	if n <= 0 {
		return nil, fmt.Errorf("rate must be positive: %d", n)
	}
	if interval <= 0 {
		return nil, fmt.Errorf("interval must be positive: %s", interval)
	}
	every := interval / time.Duration(n)
	if every <= 0 {
		return nil, fmt.Errorf("rate interval rounds to zero: %d per %s", n, interval)
	}
	return rate.NewLimiter(rate.Every(every), 1), nil
}

// RPS returns a blocking limiter for n requests per second.
func RPS(n int) (*rate.Limiter, error) {
	return NewLimiter(n, time.Second)
}

// Gate bounds concurrent request execution.
type Gate interface {
	Wait(context.Context) error
	Release()
}

// Semaphore is a small context-aware concurrency gate.
type Semaphore struct {
	ch chan struct{}
}

// NewSemaphore creates a concurrency gate with n slots.
func NewSemaphore(n int) (*Semaphore, error) {
	if n <= 0 {
		return nil, fmt.Errorf("semaphore size must be positive: %d", n)
	}
	return &Semaphore{ch: make(chan struct{}, n)}, nil
}

// Wait acquires one slot or returns ctx.Err.
func (s *Semaphore) Wait(ctx context.Context) error {
	if s == nil {
		return nil
	}
	select {
	case s.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release releases one slot.
func (s *Semaphore) Release() {
	if s == nil {
		return
	}
	select {
	case <-s.ch:
	default:
	}
}
