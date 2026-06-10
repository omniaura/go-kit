// Package shutdown coordinates graceful process shutdown: a Context carries a
// cancelable background context and a WaitGroup, Run launches tracked
// goroutines against it (optionally bounded by the shutdown timeout), and
// Shutdown cancels then waits for every tracked task to finish.
package shutdown

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type state struct {
	cancel context.CancelFunc
	once   sync.Once
}

type Context struct {
	Background       context.Context
	WaitGroup        *sync.WaitGroup
	ShutdownDuration time.Duration
	activeTasks      *atomic.Int64
	state            *state
}

type RunOption func(*runOptions)

type runOptions struct {
	withTimeout bool
	timeout     time.Duration
}

func NewContext(parent context.Context, shutdownDuration time.Duration) Context {
	ctx, cancel := context.WithCancel(parent)
	return Context{
		Background:       ctx,
		WaitGroup:        &sync.WaitGroup{},
		ShutdownDuration: shutdownDuration,
		activeTasks:      &atomic.Int64{},
		state:            &state{cancel: cancel},
	}
}

func New(parent context.Context, shutdownDuration time.Duration) Context {
	return NewContext(parent, shutdownDuration)
}

func WithoutTimeout() RunOption {
	return func(opts *runOptions) {
		opts.withTimeout = false
	}
}

func WithTimeout(timeout time.Duration) RunOption {
	return func(opts *runOptions) {
		opts.withTimeout = true
		opts.timeout = timeout
	}
}

func (s Context) Run(f func(ctx context.Context), opts ...RunOption) {
	cfg := runOptions{
		withTimeout: true,
		timeout:     s.ShutdownDuration,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	s.WaitGroup.Add(1)
	if s.activeTasks != nil {
		s.activeTasks.Add(1)
	}
	go func() {
		ctx := s.Background
		cancel := func() {}
		if cfg.withTimeout && cfg.timeout > 0 {
			ctx, cancel = context.WithTimeout(s.Background, cfg.timeout)
		}
		defer func() {
			cancel()
			if s.activeTasks != nil {
				s.activeTasks.Add(-1)
			}
			s.WaitGroup.Done()
		}()
		f(ctx)
	}()
}

func (s Context) Wait() {
	s.WaitGroup.Wait()
}

func (s Context) ActiveCount() int64 {
	if s.activeTasks == nil {
		return 0
	}
	return s.activeTasks.Load()
}

func (s Context) Cancel() {
	if s.state == nil || s.state.cancel == nil {
		return
	}
	s.state.once.Do(s.state.cancel)
}

func (s Context) Shutdown() {
	s.Cancel()
	s.Wait()
}
