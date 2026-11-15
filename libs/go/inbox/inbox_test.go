package inbox_test

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sokolovlev/polka/libs/go/inbox"
	"github.com/sokolovlev/polka/libs/go/outbox"
	"github.com/sokolovlev/polka/libs/go/pgtest"
)

func migrations(t *testing.T) fs.FS {
	t.Helper()
	schema, err := fs.ReadFile(outbox.Schema, "00001_outbox_inbox.sql")
	require.NoError(t, err)
	return fstest.MapFS{
		"00001_outbox.sql": {Data: schema},
		"00002_counter.sql": {Data: []byte(`-- +goose Up
CREATE TABLE counter (id int PRIMARY KEY, n int NOT NULL);
INSERT INTO counter VALUES (1, 0);
-- +goose Down
DROP TABLE counter;
`)},
	}
}

func increment(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "UPDATE counter SET n = n + 1 WHERE id = 1")
	return err
}

func counter(ctx context.Context, t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(ctx, "SELECT n FROM counter WHERE id = 1").Scan(&n))
	return n
}

func TestDuplicateIsSkipped(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, migrations(t))
	id := uuid.Must(uuid.NewV7())

	processed, err := inbox.Handle(ctx, pool, id, increment)
	require.NoError(t, err)
	assert.True(t, processed)

	processed, err = inbox.Handle(ctx, pool, id, increment)
	require.NoError(t, err)
	assert.False(t, processed, "the same event must not be applied twice")
	assert.Equal(t, 1, counter(ctx, t, pool))

	processed, err = inbox.Handle(ctx, pool, uuid.Must(uuid.NewV7()), increment)
	require.NoError(t, err)
	assert.True(t, processed, "another event is processed")
	assert.Equal(t, 2, counter(ctx, t, pool))
}

func TestFailedHandlerLeavesNoTrace(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, migrations(t))
	id := uuid.Must(uuid.NewV7())
	boom := errors.New("boom")

	_, err := inbox.Handle(ctx, pool, id, func(ctx context.Context, tx pgx.Tx) error {
		if err := increment(ctx, tx); err != nil {
			return err
		}
		return boom
	})
	require.ErrorIs(t, err, boom)
	assert.Equal(t, 0, counter(ctx, t, pool), "fn's change must be rolled back")

	// The redelivery is processed: the failed attempt did not record the id.
	processed, err := inbox.Handle(ctx, pool, id, increment)
	require.NoError(t, err)
	assert.True(t, processed)
	assert.Equal(t, 1, counter(ctx, t, pool))
}

func TestConcurrentDuplicatesApplyOnce(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, migrations(t))
	id := uuid.Must(uuid.NewV7())

	const workers = 10
	var applied atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			processed, err := inbox.Handle(ctx, pool, id, increment)
			assert.NoError(t, err)
			if processed {
				applied.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.EqualValues(t, 1, applied.Load())
	assert.Equal(t, 1, counter(ctx, t, pool))
}
