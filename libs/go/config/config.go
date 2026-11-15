// Package config loads service configuration from environment variables and
// validates it at startup (12-factor, spec 8.4): a service with bad config
// fails fast instead of failing on the first request.
package config

import (
	"errors"
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Validator is implemented by configs with checks beyond required/parse.
type Validator interface {
	Validate() error
}

// Load parses environment variables into T using `env` struct tags
// (github.com/caarlos0/env). Fields without a default are required. If T
// implements Validator, Validate is called after parsing.
func Load[T any]() (T, error) {
	var cfg T
	if err := env.ParseWithOptions(&cfg, env.Options{RequiredIfNoDef: true}); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	if v, ok := any(&cfg).(Validator); ok {
		if err := v.Validate(); err != nil {
			return cfg, fmt.Errorf("invalid config: %w", err)
		}
	}
	return cfg, nil
}

// Common is embedded in every Go service config.
type Common struct {
	// Address of the admin HTTP server: /healthz, /readyz, /version.
	AdminAddr string `env:"ADMIN_ADDR" envDefault:":8081"`
	// Log level: debug, info, warn, error.
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	// Seconds to finish in-flight work after SIGTERM (spec 8.2: up to 25 s).
	ShutdownTimeoutSec int `env:"SHUTDOWN_TIMEOUT_SEC" envDefault:"25"`
}

// Validate checks the common settings.
func (c Common) Validate() error {
	var errs []error
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL: unknown level %q", c.LogLevel))
	}
	if c.ShutdownTimeoutSec < 1 || c.ShutdownTimeoutSec > 25 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT_SEC: %d not in 1..25", c.ShutdownTimeoutSec))
	}
	return errors.Join(errs...)
}

// GRPC is embedded in configs of services that serve gRPC.
type GRPC struct {
	GRPCAddr string `env:"GRPC_ADDR" envDefault:":9090"`
	// Server reflection, for grpcurl in development only (spec 6.1).
	GRPCReflection bool `env:"GRPC_REFLECTION" envDefault:"false"`
}
