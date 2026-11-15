// Package grpcapi is the gRPC transport layer of catalog. Methods are added in the
// stage that needs them (spec 6.2); until then every call returns
// UNIMPLEMENTED from the embedded Unimplemented*Server.
package grpcapi

import (
	"google.golang.org/grpc"

	catalogv1 "github.com/sokolovlev/polka/libs/go/proto/polka/catalog/v1"
)

// catalogServiceServer implements catalogv1.CatalogServiceServer.
type catalogServiceServer struct {
	catalogv1.UnimplementedCatalogServiceServer
}

// Register adds the catalog gRPC services to s.
func Register(s *grpc.Server) {
	catalogv1.RegisterCatalogServiceServer(s, catalogServiceServer{})
}
