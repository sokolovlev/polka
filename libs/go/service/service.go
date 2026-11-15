// Package service wires the common parts of a Go gRPC service: config,
// logger, gRPC server with health, admin HTTP server and graceful shutdown.
// A service's main stays a few lines; it outgrows this helper when it gains
// dependencies (database, Kafka), which add readiness checks and components.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"

	"github.com/sokolovlev/polka/libs/go/admin"
	"github.com/sokolovlev/polka/libs/go/buildinfo"
	"github.com/sokolovlev/polka/libs/go/config"
	"github.com/sokolovlev/polka/libs/go/grpcserver"
	"github.com/sokolovlev/polka/libs/go/lifecycle"
	"github.com/sokolovlev/polka/libs/go/logging"
)

// GRPCConfig is the configuration of a gRPC service without dependencies.
type GRPCConfig struct {
	config.Common
	config.GRPC
}

// Main runs a gRPC service and returns the process exit code. register adds
// the service implementations to the server.
func Main(name string, register func(*grpc.Server)) int {
	if admin.IsHealthcheck(os.Args) {
		return probe()
	}
	cfg, err := config.Load[GRPCConfig]()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	log := logging.New(name, cfg.LogLevel)
	if err := RunGRPC(context.Background(), name, cfg, log, register); err != nil {
		log.Error("service stopped with error", slog.Any("error", err))
		return 1
	}
	return 0
}

// RunGRPC binds the configured addresses and serves until ctx is cancelled
// or a signal arrives.
func RunGRPC(ctx context.Context, name string, cfg GRPCConfig, log *slog.Logger, register func(*grpc.Server)) error {
	lc := net.ListenConfig{}
	grpcLn, err := lc.Listen(ctx, "tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen grpc %s: %w", cfg.GRPCAddr, err)
	}
	adminLn, err := lc.Listen(ctx, "tcp", cfg.AdminAddr)
	if err != nil {
		_ = grpcLn.Close()
		return fmt.Errorf("listen admin %s: %w", cfg.AdminAddr, err)
	}
	return ServeGRPC(ctx, name, cfg, log, Listeners{GRPC: grpcLn, Admin: adminLn}, register)
}

// Listeners are already bound sockets for ServeGRPC.
type Listeners struct {
	GRPC  net.Listener
	Admin net.Listener
}

// ServeGRPC serves on the given listeners until ctx is cancelled or a signal
// arrives. Addresses in cfg are ignored.
func ServeGRPC(ctx context.Context, name string, cfg GRPCConfig, log *slog.Logger, ls Listeners, register func(*grpc.Server)) error {
	ready := &lifecycle.Readiness{}
	gs := grpcserver.NewFromListener(ls.GRPC, cfg.GRPCReflection)
	register(gs.Server)
	adm := admin.NewHTTPServerFromListener("admin", ls.Admin, admin.Handler(buildinfo.Get(name), ready))
	log.Info("listening", slog.String("grpc", gs.Addr()), slog.String("admin", adm.Addr()),
		slog.String("version", buildinfo.Version), slog.String("commit", buildinfo.Commit))

	r := lifecycle.Runner{
		Log:             log,
		ShutdownTimeout: time.Duration(cfg.ShutdownTimeoutSec) * time.Second,
		Readiness:       ready,
		OnDrain:         gs.Drain,
	}
	return r.Run(ctx, gs, adm) //nolint:wrapcheck // errors are wrapped per component
}

func probe() int {
	addr := os.Getenv("ADMIN_ADDR")
	if addr == "" {
		addr = ":8081"
	}
	if err := admin.Probe(context.Background(), addr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
