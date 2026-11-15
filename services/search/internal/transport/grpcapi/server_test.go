package grpcapi_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	searchv1 "github.com/sokolovlev/polka/libs/go/proto/polka/search/v1"

	"github.com/sokolovlev/polka/services/search/internal/transport/grpcapi"
)

func TestRegister(t *testing.T) {
	s := grpc.NewServer()
	grpcapi.Register(s)
	info := s.GetServiceInfo()
	assert.Contains(t, info, searchv1.SearchService_ServiceDesc.ServiceName)
}
