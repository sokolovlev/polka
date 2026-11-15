// Package pg connects Go services to PostgreSQL (pgx pool), runs goose
// migrations and wraps transactions. Each service has its own database and
// role (spec 7); this package knows nothing about schemas.
package pg

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/sokolovlev/polka/libs/go/admin"
)

// Config is embedded in configs of services with a database.
type Config struct {
	DatabaseURL string `env:"DATABASE_URL"`
	// Upper bound of pool connections per replica.
	DBMaxConns int32 `env:"DB_MAX_CONNS" envDefault:"10"`
}

const connectTimeout = 5 * time.Second

// Connect opens a pool and pings the database, so a wrong URL or password
// fails at startup.
func Connect(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	pc, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	if cfg.DBMaxConns > 0 {
		pc.MaxConns = cfg.DBMaxConns
	}
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Check is the readiness check for the database.
func Check(pool *pgxpool.Pool) admin.Check {
	return admin.Check{Name: "postgres", Check: func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			return fmt.Errorf("ping: %w", err)
		}
		return nil
	}}
}

// Migrate applies all pending goose migrations found at the root of
// migrations. Services embed their SQL files and pass
// fs.Sub(embedded, "migrations"), so the binary carries its own schema and
// `service migrate` needs no extra files.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrations fs.FS) error {
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations,
		goose.WithDisableGlobalRegistry(true))
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// InTx runs fn in a transaction: commit if fn returns nil, rollback
// otherwise (and on panic). This is the unit in which a service changes its
// data and writes the outbox message together.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
			}
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
