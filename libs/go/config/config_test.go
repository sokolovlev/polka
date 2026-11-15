package config_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sokolovlev/polka/libs/go/config"
)

type testConfig struct {
	config.Common
	config.GRPC
	DatabaseURL string `env:"DATABASE_URL"`
	MaxItems    int    `env:"MAX_ITEMS" envDefault:"10"`
}

func (c testConfig) Validate() error {
	if c.MaxItems > 100 {
		return errors.New("MAX_ITEMS: must be <= 100")
	}
	return c.Common.Validate()
}

func TestLoadDefaultsAndRequired(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")

	cfg, err := config.Load[testConfig]()
	require.NoError(t, err)
	assert.Equal(t, ":8081", cfg.AdminAddr)
	assert.Equal(t, ":9090", cfg.GRPCAddr)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 25, cfg.ShutdownTimeoutSec)
	assert.Equal(t, 10, cfg.MaxItems)
	assert.Equal(t, "postgres://x", cfg.DatabaseURL)
}

func TestLoadFailsWithoutRequired(t *testing.T) {
	_, err := config.Load[testConfig]()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL")
}

func TestLoadRunsValidate(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("MAX_ITEMS", "500")

	_, err := config.Load[testConfig]()
	require.ErrorContains(t, err, "MAX_ITEMS")
}

func TestCommonValidate(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("LOG_LEVEL", "verbose")
	t.Setenv("SHUTDOWN_TIMEOUT_SEC", "60")

	_, err := config.Load[testConfig]()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LOG_LEVEL")
	assert.Contains(t, err.Error(), "SHUTDOWN_TIMEOUT_SEC")
}

func TestLoadParseError(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("MAX_ITEMS", "ten")

	_, err := config.Load[testConfig]()
	// caarlos0/env names the struct field in parse errors
	require.ErrorContains(t, err, "MaxItems")
}
