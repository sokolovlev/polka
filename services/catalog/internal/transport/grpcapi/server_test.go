package grpcapi_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	catalogv1 "github.com/sokolovlev/polka/libs/go/proto/polka/catalog/v1"

	"github.com/sokolovlev/polka/services/catalog/internal/transport/grpcapi"
)

func TestRegister(t *testing.T) {
	s := grpc.NewServer()
	grpcapi.Register(s)
	info := s.GetServiceInfo()
	assert.Contains(t, info, catalogv1.CatalogService_ServiceDesc.ServiceName)
}
