package pg_test

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sokolovlev/polka/libs/go/pg"
	"github.com/sokolovlev/polka/libs/go/pgtest"
)

var migrations = fstest.MapFS{
	"00001_items.sql": {Data: []byte(`-- +goose Up
CREATE TABLE items (id int PRIMARY KEY, name text NOT NULL);
-- +goose Down
DROP TABLE items;
`)},
}

func count(ctx context.Context, t *testing.T, q pgx.Tx) int {
	t.Helper()
	var n int
	require.NoError(t, q.QueryRow(ctx, "SELECT count(*) FROM items").Scan(&n))
	return n
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, migrations)

	// pgtest already migrated once; a second run applies nothing.
	require.NoError(t, pg.Migrate(ctx, pool, migrations))
	var version int64
	require.NoError(t, pool.QueryRow(ctx, "SELECT max(version_id) FROM goose_db_version").Scan(&version))
	assert.EqualValues(t, 1, version)
}

func TestInTxCommitsAndRollsBack(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, migrations)

	require.NoError(t, pg.InTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO items VALUES (1, 'a')")
		return err
	}))

	boom := errors.New("boom")
	err := pg.InTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO items VALUES (2, 'b')"); err != nil {
			return err
		}
		return boom
	})
	require.ErrorIs(t, err, boom)

	require.NoError(t, pg.InTx(ctx, pool, func(tx pgx.Tx) error {
		assert.Equal(t, 1, count(ctx, t, tx), "the failed transaction must leave nothing")
		return nil
	}))
}

func TestInTxRollsBackOnPanic(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, migrations)

	assert.Panics(t, func() {
		_ = pg.InTx(ctx, pool, func(tx pgx.Tx) error {
			_, _ = tx.Exec(ctx, "INSERT INTO items VALUES (3, 'c')")
			panic("bug")
		})
	})
	require.NoError(t, pg.InTx(ctx, pool, func(tx pgx.Tx) error {
		assert.Equal(t, 0, count(ctx, t, tx))
		return nil
	}))
}

func TestEachTestGetsItsOwnDatabase(t *testing.T) {
	ctx := context.Background()
	a := pgtest.New(t, migrations)
	b := pgtest.New(t, migrations)
	_, err := a.Exec(ctx, "INSERT INTO items VALUES (1, 'a')")
	require.NoError(t, err)
	var n int
	require.NoError(t, b.QueryRow(ctx, "SELECT count(*) FROM items").Scan(&n))
	assert.Zero(t, n)
}

func TestCheck(t *testing.T) {
	pool := pgtest.New(t, nil)
	require.NoError(t, pg.Check(pool).Check(context.Background()))
	pool.Close()
	require.Error(t, pg.Check(pool).Check(context.Background()))
}

func TestConnectFailsFast(t *testing.T) {
	_, err := pg.Connect(context.Background(), pg.Config{DatabaseURL: "postgres://u:p@127.0.0.1:1/db"})
	require.Error(t, err)
	_, err = pg.Connect(context.Background(), pg.Config{DatabaseURL: "::not a url"})
	require.ErrorContains(t, err, "parse DATABASE_URL")
}
