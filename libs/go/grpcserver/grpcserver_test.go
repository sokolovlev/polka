package grpcserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	orderv1 "github.com/sokolovlev/polka/libs/go/proto/polka/order/v1"

	"github.com/sokolovlev/polka/libs/go/grpcserver"
)

type orderServer struct {
	orderv1.UnimplementedOrderServiceServer
}

func TestHealthServingThenDraining(t *testing.T) {
	srv, err := grpcserver.New(context.Background(), "127.0.0.1:0", false)
	require.NoError(t, err)
	orderv1.RegisterOrderServiceServer(srv, orderServer{})
	done := make(chan error, 1)
	go func() { done <- srv.Run(context.Background()) }()

	conn, err := grpc.NewClient(srv.Addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := healthpb.NewHealthClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, svc := range []string{"", orderv1.OrderService_ServiceDesc.ServiceName} {
		var resp *healthpb.HealthCheckResponse
		require.Eventually(t, func() bool {
			resp, err = client.Check(ctx, &healthpb.HealthCheckRequest{Service: svc})
			return err == nil
		}, 2*time.Second, 20*time.Millisecond)
		assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus(), "service %q", svc)
	}

	srv.Drain()
	resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, resp.GetStatus())

	require.NoError(t, srv.Shutdown(ctx))
	require.NoError(t, <-done)
}
