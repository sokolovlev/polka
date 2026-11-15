package grpcclient_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/sokolovlev/polka/libs/go/grpcclient"
)

// invoke runs the interceptor with a fake invoker that reports the deadline
// it saw.
func invoke(ctx context.Context, t *testing.T, timeout time.Duration) (time.Time, bool) {
	t.Helper()
	var deadline time.Time
	var has bool
	err := grpcclient.DeadlineInterceptor(timeout)(ctx, "/svc/Method", nil, nil, nil,
		func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			deadline, has = ctx.Deadline()
			return nil
		})
	require.NoError(t, err)
	return deadline, has
}

func TestAddsDeadlineWhenMissing(t *testing.T) {
	start := time.Now()
	deadline, has := invoke(context.Background(), t, 2*time.Second)
	require.True(t, has)
	assert.WithinDuration(t, start.Add(2*time.Second), deadline, 100*time.Millisecond)
}

func TestKeepsCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	want, _ := ctx.Deadline()

	got, has := invoke(ctx, t, 2*time.Second)
	require.True(t, has)
	assert.Equal(t, want, got, "a caller's own deadline wins, even if longer")
}

func TestDialIsLazy(t *testing.T) {
	conn, err := grpcclient.Dial("127.0.0.1:1")
	require.NoError(t, err, "dialing must not fail when the service is down")
	require.NoError(t, conn.Close())
}
