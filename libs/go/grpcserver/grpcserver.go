// Package grpcserver builds the gRPC server every Go service uses, with the
// standard health service (grpc.health.v1, used by the Kubernetes gRPC probe)
// and optional reflection for development (spec 6.1).
package grpcserver

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// Server is a gRPC server implementing lifecycle.Component.
type Server struct {
	*grpc.Server
	health *health.Server
	ln     net.Listener
}

// New binds addr and creates a server with health (and, if enabled,
// reflection) registered. Register service implementations on the embedded
// *grpc.Server before Run.
func New(ctx context.Context, addr string, reflectionOn bool, opts ...grpc.ServerOption) (*Server, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	return NewFromListener(ln, reflectionOn, opts...), nil
}

// NewFromListener is New on an already bound listener (tests bind
// 127.0.0.1:0 themselves, so no other process can take the port meanwhile).
func NewFromListener(ln net.Listener, reflectionOn bool, opts ...grpc.ServerOption) *Server {
	s := &Server{Server: grpc.NewServer(opts...), health: health.NewServer(), ln: ln}
	healthpb.RegisterHealthServer(s.Server, s.health)
	if reflectionOn {
		reflection.Register(s.Server)
	}
	return s
}

// Addr is the bound address.
func (s *Server) Addr() string { return s.ln.Addr().String() }

// Name implements lifecycle.Component.
func (s *Server) Name() string { return "grpc" }

// Run marks every registered service SERVING and serves until stopped.
func (s *Server) Run(context.Context) error {
	s.health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	for name := range s.GetServiceInfo() {
		s.health.SetServingStatus(name, healthpb.HealthCheckResponse_SERVING)
	}
	if err := s.Serve(s.ln); err != nil {
		return fmt.Errorf("grpc serve: %w", err)
	}
	return nil
}

// Drain reports NOT_SERVING to health checks while in-flight calls finish.
// Pass it as lifecycle.Runner.OnDrain.
func (s *Server) Drain() { s.health.Shutdown() }

// Shutdown waits for in-flight RPCs (GracefulStop) until ctx expires, then
// closes remaining connections.
func (s *Server) Shutdown(ctx context.Context) error {
	s.health.Shutdown()
	done := make(chan struct{})
	go func() {
		s.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		s.Stop()
		return fmt.Errorf("grpc graceful stop: %w", ctx.Err())
	}
}
