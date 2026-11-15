// Package kvtest gives integration tests a real Valkey (testcontainers).
// One container per test binary; each New call gets its own logical
// database (0..15), flushed first. Skipped when Docker is not available.
package kvtest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/sokolovlev/polka/libs/go/kv"
)

// Image matches deploy/compose/compose.yaml.
const Image = "valkey/valkey:8.1.11-alpine"

var (
	once     sync.Once
	addr     string
	startErr error
	nextDB   atomic.Int32
)

func start() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := testcontainers.Run(ctx, Image,
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("6379/tcp").WithStartupTimeout(time.Minute)),
	)
	if err != nil {
		startErr = fmt.Errorf("start valkey container: %w", err)
		return
	}
	addr, startErr = c.PortEndpoint(ctx, "6379/tcp", "")
}

// New returns a client to an empty logical database. At most 16 per test
// binary (Valkey's default number of databases).
func New(t *testing.T) *redis.Client {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	once.Do(start)
	if startErr != nil {
		t.Fatal(startErr)
	}
	db := int(nextDB.Add(1) - 1)
	if db > 15 {
		t.Fatal("kvtest: more than 16 databases in one test binary")
	}
	ctx := context.Background()
	c, err := kv.Connect(ctx, kv.Config{ValkeyAddr: addr, ValkeyDB: db})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := c.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	return c
}
