package grpcapi_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	orderv1 "github.com/sokolovlev/polka/libs/go/proto/polka/order/v1"

	"github.com/sokolovlev/polka/services/order/internal/transport/grpcapi"
)

func TestRegister(t *testing.T) {
	s := grpc.NewServer()
	grpcapi.Register(s)
	info := s.GetServiceInfo()
	assert.Contains(t, info, orderv1.OrderService_ServiceDesc.ServiceName)
	assert.Contains(t, info, orderv1.CartService_ServiceDesc.ServiceName)
}
