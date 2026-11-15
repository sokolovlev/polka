package service_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/sokolovlev/polka/libs/go/admin"
	"github.com/sokolovlev/polka/libs/go/config"
	searchv1 "github.com/sokolovlev/polka/libs/go/proto/polka/search/v1"
	"github.com/sokolovlev/polka/libs/go/service"
)

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return ln
}

type searchServer struct {
	searchv1.UnimplementedSearchServiceServer
}

// End to end: the service serves gRPC health and admin /readyz, and returns
// cleanly when its context is cancelled.
func TestRunGRPC(t *testing.T) {
	// Bound here and handed over: no free-port race.
	ls := service.Listeners{GRPC: listen(t), Admin: listen(t)}
	grpcAddr, adminAddr := ls.GRPC.Addr().String(), ls.Admin.Addr().String()
	cfg := service.GRPCConfig{Common: config.Common{LogLevel: "info", ShutdownTimeoutSec: 5}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- service.ServeGRPC(ctx, "search", cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), ls,
			func(s *grpc.Server) { searchv1.RegisterSearchServiceServer(s, searchServer{}) })
	}()

	require.Eventually(t, func() bool {
		return admin.Probe(context.Background(), adminAddr) == nil
	}, 3*time.Second, 20*time.Millisecond)

	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	resp, err := healthpb.NewHealthClient(conn).Check(context.Background(),
		&healthpb.HealthCheckRequest{Service: searchv1.SearchService_ServiceDesc.ServiceName})
	require.NoError(t, err)
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("service did not stop")
	}
}
