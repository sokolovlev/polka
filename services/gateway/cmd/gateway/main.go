// Command gateway is the single public entry point: REST/JSON and WebSocket
// for the web client, gRPC to the services (spec 4.2).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/sokolovlev/polka/libs/go/admin"
	"github.com/sokolovlev/polka/libs/go/buildinfo"
	"github.com/sokolovlev/polka/libs/go/config"
	"github.com/sokolovlev/polka/libs/go/lifecycle"
	"github.com/sokolovlev/polka/libs/go/logging"

	"github.com/sokolovlev/polka/services/gateway/internal/transport/httpapi"
)

type gatewayConfig struct {
	config.Common
	// Public HTTP port; /healthz, /readyz and /version are served here too
	// (spec 6.5), so the gateway has no separate admin port.
	HTTPAddr string `env:"HTTP_ADDR" envDefault:":8080"`
}

func main() {
	os.Exit(run())
}

func run() int {
	if admin.IsHealthcheck(os.Args) {
		addr := os.Getenv("HTTP_ADDR")
		if addr == "" {
			addr = ":8080"
		}
		if err := admin.Probe(context.Background(), addr); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	cfg, err := config.Load[gatewayConfig]()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	log := logging.New("gateway", cfg.LogLevel)
	ctx := context.Background()

	ready := &lifecycle.Readiness{}
	srv, err := admin.NewHTTPServer(ctx, "http", cfg.HTTPAddr, httpapi.NewRouter(buildinfo.Get("gateway"), ready))
	if err != nil {
		log.Error("start", slog.Any("error", err))
		return 1
	}
	log.Info("listening", slog.String("http", srv.Addr()),
		slog.String("version", buildinfo.Version), slog.String("commit", buildinfo.Commit))

	r := lifecycle.Runner{
		Log:             log,
		ShutdownTimeout: time.Duration(cfg.ShutdownTimeoutSec) * time.Second,
		Readiness:       ready,
	}
	if err := r.Run(ctx, srv); err != nil {
		log.Error("service stopped with error", slog.Any("error", err))
		return 1
	}
	return 0
}
