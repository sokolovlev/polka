package grpcapi_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	authv1 "github.com/sokolovlev/polka/libs/go/proto/polka/auth/v1"

	"github.com/sokolovlev/polka/services/auth/internal/transport/grpcapi"
)

func TestRegister(t *testing.T) {
	s := grpc.NewServer()
	grpcapi.Register(s)
	info := s.GetServiceInfo()
	assert.Contains(t, info, authv1.AuthService_ServiceDesc.ServiceName)
}
