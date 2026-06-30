package tasker

import (
	"context"
	stderrs "errors"
	"fmt"
	"time"

	"github.com/omniaura/go-kit/errs"
)

// RunFunc is the function a Tasker executes.
type RunFunc[C any] func(context.Context, C) error

// Tasker runs background work with a typed context value.
type Tasker[C any] struct {
	Context     C
	Interval    time.Duration
	RetryPolicy errs.RetryPolicy

	run RunFunc[C]
}

type options struct {
	Interval    *time.Duration
	RetryPolicy *errs.RetryPolicy
}

type OptFunc func(*options) error

// WithInterval sets the interval used by RunEvery and Start.
func WithInterval(interval time.Duration) OptFunc {
	return func(o *options) error {
		if interval < 0 {
			return fmt.Errorf("interval less than 0: %s", interval)
		}
		o.Interval = &interval
		return nil
	}
}

// WithRetryPolicy sets the fallback retry policy used when an error does not
// carry retry metadata from errs.
func WithRetryPolicy(policy errs.RetryPolicy) OptFunc {
	return func(o *options) error {
		o.RetryPolicy = &policy
		return nil
	}
}

// New creates a Tasker with a typed context value and run function.
func New[C any](typedContext C, run RunFunc[C], opts ...OptFunc) (*Tasker[C], error) {
	if run == nil {
		return nil, fmt.Errorf("run function is nil")
	}

	var o options
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return nil, err
		}
	}

	t := &Tasker[C]{
		Context: typedContext,
		run:     run,
	}
	if o.Interval != nil {
		t.Interval = *o.Interval
	}
	if o.RetryPolicy != nil {
		t.RetryPolicy = *o.RetryPolicy
	}
	return t, nil
}

// Run executes the task once. If the returned error carries retry metadata
// from errs, that metadata controls bounded retries for this run.
func (t *Tasker[C]) Run(ctx context.Context) error {
	if t == nil {
		return fmt.Errorf("tasker is nil")
	}
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := t.run(ctx, t.Context)
		if err == nil {
			return nil
		}

		policy := t.RetryPolicy
		if errPolicy, ok := errs.RetryPolicyOf(err); ok {
			policy = errPolicy
		}
		if !policy.ShouldRetry(attempt) {
			return err
		}
		if err := policy.Wait(ctx, attempt); err != nil {
			return err
		}
	}
}

// RunEvery runs the task immediately, then again after each interval until the
// context is canceled or a run returns an unretryable error.
func (t *Tasker[C]) RunEvery(ctx context.Context) error {
	if t == nil {
		return fmt.Errorf("tasker is nil")
	}
	if t.Interval <= 0 {
		return fmt.Errorf("interval must be greater than 0")
	}

	for {
		if err := t.Run(ctx); err != nil {
			return err
		}

		timer := time.NewTimer(t.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Start runs RunEvery in a goroutine and returns unrecoverable task errors.
// Normal context cancellation closes the channel without sending an error.
func (t *Tasker[C]) Start(ctx context.Context) <-chan error {
	errsCh := make(chan error, 1)
	go func() {
		defer close(errsCh)
		err := t.RunEvery(ctx)
		if err == nil {
			return
		}
		if stderrs.Is(err, context.Canceled) || stderrs.Is(err, context.DeadlineExceeded) {
			return
		}
		errsCh <- err
	}()
	return errsCh
}
