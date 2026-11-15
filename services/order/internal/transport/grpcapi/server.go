// Package grpcapi is the gRPC transport layer of order. Methods are added in the
// stage that needs them (spec 6.2); until then every call returns
// UNIMPLEMENTED from the embedded Unimplemented*Server.
package grpcapi

import (
	"google.golang.org/grpc"

	orderv1 "github.com/sokolovlev/polka/libs/go/proto/polka/order/v1"
)

// orderServiceServer implements orderv1.OrderServiceServer.
type orderServiceServer struct {
	orderv1.UnimplementedOrderServiceServer
}

// cartServiceServer implements orderv1.CartServiceServer.
type cartServiceServer struct {
	orderv1.UnimplementedCartServiceServer
}

// Register adds the order gRPC services to s.
func Register(s *grpc.Server) {
	orderv1.RegisterOrderServiceServer(s, orderServiceServer{})
	orderv1.RegisterCartServiceServer(s, cartServiceServer{})
}
