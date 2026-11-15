// Package kv connects Go services to Valkey (Redis protocol, R9) with
// go-redis. Keys and TTLs are owned by each service (spec 7.2).
package kv

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/sokolovlev/polka/libs/go/admin"
)

// Config is embedded in configs of services that use Valkey.
type Config struct {
	ValkeyAddr string `env:"VALKEY_ADDR" envDefault:"valkey:6379"`
	ValkeyDB   int    `env:"VALKEY_DB" envDefault:"0"`
}

// Connect creates a client and pings it, so a wrong address fails at startup.
func Connect(ctx context.Context, cfg Config) (*redis.Client, error) {
	c := redis.NewClient(&redis.Options{
		Addr:         cfg.ValkeyAddr,
		DB:           cfg.ValkeyDB,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	})
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("ping valkey %s: %w", cfg.ValkeyAddr, err)
	}
	return c, nil
}

// Check is the readiness check for Valkey.
func Check(c *redis.Client) admin.Check {
	return admin.Check{Name: "valkey", Check: func(ctx context.Context) error {
		if err := c.Ping(ctx).Err(); err != nil {
			return fmt.Errorf("ping: %w", err)
		}
		return nil
	}}
}
