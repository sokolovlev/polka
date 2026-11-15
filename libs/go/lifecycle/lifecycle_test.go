package lifecycle

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeComponent blocks in Run until Shutdown is called or fails on demand.
type fakeComponent struct {
	name       string
	runErr     error
	stopped    chan struct{}
	shutdowns  atomic.Int32
	shutdownFn func(ctx context.Context) error
}

func newFake(name string) *fakeComponent {
	return &fakeComponent{name: name, stopped: make(chan struct{})}
}

func (f *fakeComponent) Name() string { return f.name }

func (f *fakeComponent) Run(context.Context) error {
	if f.runErr != nil {
		return f.runErr
	}
	<-f.stopped
	return nil
}

func (f *fakeComponent) Shutdown(ctx context.Context) error {
	if f.shutdowns.Add(1) == 1 && f.runErr == nil {
		defer close(f.stopped)
	}
	if f.shutdownFn != nil {
		return f.shutdownFn(ctx)
	}
	return nil
}

func testRunner(r *Readiness) Runner {
	return Runner{
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		ShutdownTimeout: time.Second,
		Readiness:       r,
	}
}

func TestRunStopsAllOnCancel(t *testing.T) {
	ready := &Readiness{}
	a, b := newFake("a"), newFake("b")
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- testRunner(ready).run(ctx, a, b) }()

	assert.False(t, ready.Draining())
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
	assert.True(t, ready.Draining())
	assert.EqualValues(t, 1, a.shutdowns.Load())
	assert.EqualValues(t, 1, b.shutdowns.Load())
}

func TestFailingComponentShutsDownOthers(t *testing.T) {
	failing := newFake("failing")
	failing.runErr = errors.New("boom")
	healthy := newFake("healthy")

	err := testRunner(nil).run(context.Background(), failing, healthy)

	require.ErrorContains(t, err, "failing: boom")
	assert.EqualValues(t, 1, healthy.shutdowns.Load())
}

func TestShutdownHasDeadline(t *testing.T) {
	slow := newFake("slow")
	var deadlineSet atomic.Bool
	slow.shutdownFn = func(ctx context.Context) error {
		_, ok := ctx.Deadline()
		deadlineSet.Store(ok)
		<-ctx.Done() // never finishes on its own
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := testRunner(nil)
	r.ShutdownTimeout = 50 * time.Millisecond

	done := make(chan error, 1)
	go func() { done <- r.run(ctx, slow) }()
	cancel()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown deadline not enforced")
	}
	assert.True(t, deadlineSet.Load())
}

func TestOnDrainCalledBeforeShutdown(t *testing.T) {
	var drained atomic.Bool
	c := newFake("c")
	c.shutdownFn = func(context.Context) error {
		assert.True(t, drained.Load(), "OnDrain must run before Shutdown")
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := testRunner(nil)
	r.OnDrain = func() { drained.Store(true) }
	cancel()

	require.NoError(t, r.run(ctx, c))
}
