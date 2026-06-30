package tasker_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/omniaura/go-kit/errs"
	"github.com/omniaura/go-kit/tasker"
)

type testContext struct {
	Prefix string
}

func TestTaskerRunPassesTypedContext(t *testing.T) {
	tkr, err := tasker.New(testContext{Prefix: "db"}, func(ctx context.Context, tc testContext) error {
		if ctx == nil {
			t.Fatal("expected context")
		}
		if tc.Prefix != "db" {
			t.Fatalf("expected typed context, got %+v", tc)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := tkr.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskerRunRetriesWithErrorPolicy(t *testing.T) {
	var calls atomic.Int64
	policy := errs.FixedRetry(3, time.Millisecond)
	retryErr := errs.NewFactory(
		http.StatusServiceUnavailable,
		"temporary",
		errs.WithRetryPolicy(policy),
	).New(context.Background())

	tkr, err := tasker.New(testContext{}, func(context.Context, testContext) error {
		if calls.Add(1) < 3 {
			return retryErr
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := tkr.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected 3 calls, got %d", got)
	}
}

func TestTaskerRunUsesFallbackRetryPolicy(t *testing.T) {
	var calls atomic.Int64
	expectedErr := errors.New("temporary")

	tkr, err := tasker.New(
		testContext{},
		func(context.Context, testContext) error {
			if calls.Add(1) < 2 {
				return expectedErr
			}
			return nil
		},
		tasker.WithRetryPolicy(errs.FixedRetry(2, time.Millisecond)),
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := tkr.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}

func TestTaskerRunErrorPolicyOverridesFallbackPolicy(t *testing.T) {
	var calls atomic.Int64
	errPolicy := errs.NeverRetry()
	retryErr := errs.NewFactory(
		http.StatusBadRequest,
		"bad request",
		errs.WithRetryPolicy(errPolicy),
	).New(context.Background())

	tkr, err := tasker.New(
		testContext{},
		func(context.Context, testContext) error {
			calls.Add(1)
			return retryErr
		},
		tasker.WithRetryPolicy(errs.FixedRetry(3, time.Millisecond)),
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := tkr.Run(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
}

func TestTaskerRunEveryRunsImmediatelyAndThenOnInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int64

	tkr, err := tasker.New(
		testContext{},
		func(context.Context, testContext) error {
			if calls.Add(1) == 2 {
				cancel()
			}
			return nil
		},
		tasker.WithInterval(time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := tkr.RunEvery(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}

func TestTaskerStartSuppressesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tkr, err := tasker.New(
		testContext{},
		func(context.Context, testContext) error {
			cancel()
			return nil
		},
		tasker.WithInterval(time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}

	errsCh := tkr.Start(ctx)
	select {
	case err, ok := <-errsCh:
		if ok {
			t.Fatalf("expected closed channel, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Start to stop")
	}
}
