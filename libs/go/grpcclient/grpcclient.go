// Package grpcclient dials other Polka services. Every call gets a deadline
// (spec 6.1): if the caller set none, DefaultTimeout is applied, so a stuck
// downstream service can never hang a request forever.
package grpcclient

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// DefaultTimeout applies to calls whose context has no deadline.
const DefaultTimeout = 2 * time.Second

// Dial creates a client connection to target ("catalog:9090"). The
// connection is lazy: it does not fail if the service is down at startup;
// calls fail (and readiness reports it) instead. Plain text inside the
// cluster: mTLS is out of the MVP (spec 8.3).
func Dial(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	all := append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(DeadlineInterceptor(DefaultTimeout)),
	}, opts...)
	conn, err := grpc.NewClient(target, all...)
	if err != nil {
		return nil, fmt.Errorf("grpc client %s: %w", target, err)
	}
	return conn, nil
}

// DeadlineInterceptor applies timeout to calls without a deadline; an
// existing (shorter or longer) deadline is left as is.
func DeadlineInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
