// Package grpcapi is the gRPC transport layer of auth. Methods are added in the
// stage that needs them (spec 6.2); until then every call returns
// UNIMPLEMENTED from the embedded Unimplemented*Server.
package grpcapi

import (
	"google.golang.org/grpc"

	authv1 "github.com/sokolovlev/polka/libs/go/proto/polka/auth/v1"
)

// authServiceServer implements authv1.AuthServiceServer.
type authServiceServer struct {
	authv1.UnimplementedAuthServiceServer
}

// Register adds the auth gRPC services to s.
func Register(s *grpc.Server) {
	authv1.RegisterAuthServiceServer(s, authServiceServer{})
}
