// Package pgtest gives integration tests a real PostgreSQL (testcontainers).
//
// One container is started per test binary and shared; every call to New
// creates a fresh database in it and applies the migrations, so tests are
// isolated from each other without paying for a container per test.
// Tests are skipped when Docker is not available.
package pgtest

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/sokolovlev/polka/libs/go/pg"
)

// Image matches deploy/compose/compose.yaml.
const Image = "postgres:16.16-alpine"

var (
	once      sync.Once
	baseURL   string
	startErr  error
	dbCounter atomic.Int64
)

func start() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := postgres.Run(ctx, Image,
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.WithDatabase("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(time.Minute)),
	)
	if err != nil {
		startErr = fmt.Errorf("start postgres container: %w", err)
		return
	}
	// The container lives as long as the test binary; Ryuk removes it.
	baseURL, startErr = c.ConnectionString(ctx, "sslmode=disable")
}

// New returns a pool to a new, empty database with migrations applied
// (pass nil for none). Extensions listed in exts are created first, as the
// compose init script does (e.g. "citext", "ltree").
func New(t *testing.T, migrations fs.FS, exts ...string) *pgxpool.Pool {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	once.Do(start)
	if startErr != nil {
		t.Fatal(startErr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	name := fmt.Sprintf("test_%d", dbCounter.Add(1))
	admin, err := pg.Connect(ctx, pg.Config{DatabaseURL: baseURL})
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	pool, err := pg.Connect(ctx, pg.Config{DatabaseURL: u.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	for _, e := range exts {
		if _, err := pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+quoteIdent(e)); err != nil {
			t.Fatal(err)
		}
	}
	if migrations != nil {
		if err := pg.Migrate(ctx, pool, migrations); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
