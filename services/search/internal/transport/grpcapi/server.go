// Package grpcapi is the gRPC transport layer of search. Methods are added in the
// stage that needs them (spec 6.2); until then every call returns
// UNIMPLEMENTED from the embedded Unimplemented*Server.
package grpcapi

import (
	"google.golang.org/grpc"

	searchv1 "github.com/sokolovlev/polka/libs/go/proto/polka/search/v1"
)

// searchServiceServer implements searchv1.SearchServiceServer.
type searchServiceServer struct {
	searchv1.UnimplementedSearchServiceServer
}

// Register adds the search gRPC services to s.
func Register(s *grpc.Server) {
	searchv1.RegisterSearchServiceServer(s, searchServiceServer{})
}
