package admin_test

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sokolovlev/polka/libs/go/admin"
	"github.com/sokolovlev/polka/libs/go/buildinfo"
	"github.com/sokolovlev/polka/libs/go/lifecycle"
)

func TestIsHealthcheck(t *testing.T) {
	assert.True(t, admin.IsHealthcheck([]string{"/service", "healthcheck"}))
	assert.False(t, admin.IsHealthcheck([]string{"/service"}))
	assert.False(t, admin.IsHealthcheck([]string{"/service", "serve"}))
}

func TestProbe(t *testing.T) {
	ctx := context.Background()
	ready := &lifecycle.Readiness{}
	srv, err := admin.NewHTTPServer(ctx, "admin", "127.0.0.1:0", admin.Handler(buildinfo.Get("svc"), ready))
	require.NoError(t, err)
	go func() { _ = srv.Run(ctx) }()
	t.Cleanup(func() { _ = srv.Shutdown(ctx) })

	_, port, err := net.SplitHostPort(srv.Addr())
	require.NoError(t, err)

	// ":port" is how services configure ADMIN_ADDR.
	require.NoError(t, admin.Probe(ctx, ":"+port))

	require.Error(t, admin.Probe(ctx, "127.0.0.1:1"), "nothing listens on port 1")
	require.Error(t, admin.Probe(ctx, "no-port"))
}

func TestProbeFailsWhenNotReady(t *testing.T) {
	ctx := context.Background()
	srv, err := admin.NewHTTPServer(ctx, "admin", "127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	require.NoError(t, err)
	go func() { _ = srv.Run(ctx) }()
	t.Cleanup(func() { _ = srv.Shutdown(ctx) })

	require.ErrorContains(t, admin.Probe(ctx, srv.Addr()), "status 503")
}
