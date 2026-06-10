package shutdown

import (
	"context"
	"testing"
	"time"
)

func TestRunWithoutTimeoutDoesNotAddDeadline(t *testing.T) {
	t.Parallel()

	sd := NewContext(context.Background(), time.Nanosecond)
	defer sd.Shutdown()

	done := make(chan struct{})
	sd.Run(func(ctx context.Context) {
		defer close(done)
		if _, ok := ctx.Deadline(); ok {
			t.Fatal("Run without timeout should not add a deadline")
		}
	}, WithoutTimeout())

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not execute")
	}
}

func TestShutdownCancelsAndWaits(t *testing.T) {
	t.Parallel()

	sd := NewContext(context.Background(), time.Second)

	done := make(chan struct{})
	sd.Run(func(ctx context.Context) {
		defer close(done)
		<-ctx.Done()
	})

	sd.Shutdown()

	select {
	case <-done:
	default:
		t.Fatal("Shutdown should wait for background work to finish")
	}
}

func TestActiveCountTracksRunTasks(t *testing.T) {
	t.Parallel()

	sd := NewContext(context.Background(), time.Second)
	defer sd.Shutdown()

	release := make(chan struct{})
	started := make(chan struct{})
	sd.Run(func(ctx context.Context) {
		close(started)
		<-release
	}, WithoutTimeout())

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Run did not start")
	}

	if got, want := sd.ActiveCount(), int64(1); got != want {
		t.Fatalf("ActiveCount = %d, want %d", got, want)
	}

	close(release)
	sd.Wait()

	if got := sd.ActiveCount(); got != 0 {
		t.Fatalf("ActiveCount = %d, want 0", got)
	}
}
