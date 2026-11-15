package outbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/sokolovlev/polka/libs/go/pg"
	"github.com/sokolovlev/polka/libs/go/pgtest"
	commonv1 "github.com/sokolovlev/polka/libs/go/proto/polka/common/v1"
)

// fakePublisher records what it was asked to publish. failAt >= 0 makes the
// call that reaches that global position deliver only the prefix before it.
type fakePublisher struct {
	mu        sync.Mutex
	published []Message
	failAt    int
	delay     time.Duration
	inFlight  int
	maxFlight int
}

func newFake() *fakePublisher { return &fakePublisher{failAt: -1} }

func (f *fakePublisher) Publish(_ context.Context, msgs []Message) (int, error) {
	f.mu.Lock()
	f.inFlight++
	f.maxFlight = max(f.maxFlight, f.inFlight)
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.inFlight--; f.mu.Unlock() }()
	time.Sleep(f.delay)

	f.mu.Lock()
	defer f.mu.Unlock()
	for i, m := range msgs {
		if f.failAt >= 0 && len(f.published) == f.failAt {
			f.failAt = -1 // fail once
			return i, errors.New("broker unavailable")
		}
		f.published = append(f.published, m)
	}
	return len(msgs), nil
}

func (f *fakePublisher) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.published))
	for i, m := range f.published {
		out[i] = m.Key
	}
	return out
}

func newRelay(pool *pgxpool.Pool, pub Publisher, batch int) *Relay {
	return &Relay{
		Pool: pool, Publisher: pub, LockKey: 42, BatchSize: batch,
		Interval: 10 * time.Millisecond,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// seed writes n messages with keys "k0".."k{n-1}" in one transaction.
func seed(ctx context.Context, t *testing.T, pool *pgxpool.Pool, n int) []string {
	t.Helper()
	keys := make([]string, n)
	require.NoError(t, pg.InTx(ctx, pool, func(tx pgx.Tx) error {
		for i := range n {
			m, err := NewMessage(Event{
				Topic: "test.v1", Type: "test.happened", AggregateID: fmt.Sprintf("agg-%03d", i),
				Payload: &commonv1.Money{AmountMinor: int64(i), Currency: "RUB"},
			}, time.Now())
			if err != nil {
				return err
			}
			keys[i] = m.Key
			if err := Write(ctx, tx, m); err != nil {
				return err
			}
		}
		return nil
	}))
	return keys
}

func unpublished(ctx context.Context, t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE published_at IS NULL").Scan(&n))
	return n
}

func TestNewMessageBuildsEnvelope(t *testing.T) {
	m, err := NewMessage(Event{
		Topic: "orders.events.v1", Type: "order.created", AggregateID: "order-1",
		AggregateVersion: 3, CorrelationID: "order-1",
		Payload: &commonv1.Money{AmountMinor: 100, Currency: "RUB"},
	}, time.Unix(1763116800, 0))
	require.NoError(t, err)
	assert.Equal(t, "orders.events.v1", m.Topic)
	assert.Equal(t, "order-1", m.Key, "key = aggregate id (spec 6.3)")
	assert.EqualValues(t, 7, m.EventID.Version(), "UUIDv7")

	var env commonv1.Envelope
	require.NoError(t, proto.Unmarshal(m.Payload, &env))
	assert.Equal(t, m.EventID.String(), env.GetEventId())
	assert.Equal(t, "order.created", env.GetEventType())
	assert.EqualValues(t, 3, env.GetAggregateVersion())
	assert.Equal(t, "type.googleapis.com/polka.common.v1.Money", env.GetPayload().GetTypeUrl())
}

func TestPublishesInOrderAndMarks(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, Schema)
	keys := seed(ctx, t, pool, 5)
	pub := newFake()
	r := newRelay(pool, pub, 100)

	n, err := r.publishBatch(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, keys, pub.keys(), "published in id order")
	assert.Zero(t, unpublished(ctx, t, pool))

	n, err = r.publishBatch(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "nothing left to publish")
	assert.Len(t, pub.keys(), 5, "published rows are not sent again")
}

func TestRespectsBatchSize(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, Schema)
	seed(ctx, t, pool, 7)
	r := newRelay(pool, newFake(), 3)

	for _, want := range []int{3, 3, 1, 0} {
		n, err := r.publishBatch(ctx)
		require.NoError(t, err)
		assert.Equal(t, want, n)
	}
}

func TestMarksOnlyTheAcknowledgedPrefix(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, Schema)
	keys := seed(ctx, t, pool, 5)
	pub := newFake()
	pub.failAt = 2 // the broker fails on the third message
	r := newRelay(pool, pub, 100)

	n, err := r.publishBatch(ctx)
	require.Error(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, 3, unpublished(ctx, t, pool), "msgs[2:] stay in the outbox")

	n, err = r.publishBatch(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, keys, pub.keys(), "the retry continues from the failed message, in order")
}

func TestOneReplicaAtATime(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, Schema)
	keys := seed(ctx, t, pool, 60)
	pub := newFake()
	pub.delay = 20 * time.Millisecond

	// Three "replicas" race on the same outbox.
	var wg sync.WaitGroup
	for range 3 {
		r := newRelay(pool, pub, 10)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for unpublishedNow(ctx, pool) > 0 {
				if _, err := r.publishBatch(ctx); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, keys, pub.keys(), "every message exactly once, in global id order")
	assert.Equal(t, 1, pub.maxFlight, "never two batches in flight at once")
}

func unpublishedNow(ctx context.Context, pool *pgxpool.Pool) int {
	var n int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE published_at IS NULL").Scan(&n)
	return n
}

func TestRunAndShutdown(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, Schema)
	r := newRelay(pool, newFake(), 100)
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()

	seed(ctx, t, pool, 3)
	require.Eventually(t, func() bool { return unpublishedNow(ctx, pool) == 0 }, 5*time.Second, 20*time.Millisecond)

	sctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	require.NoError(t, r.Shutdown(sctx))
	require.NoError(t, <-done)
}

func TestCleanupDeletesOldPublishedRows(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, Schema)
	seed(ctx, t, pool, 3)
	_, err := pool.Exec(ctx, `UPDATE outbox SET published_at = now() - interval '8 days' WHERE id = (SELECT min(id) FROM outbox)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = (SELECT max(id) FROM outbox)`)
	require.NoError(t, err)

	deleted, err := newRelay(pool, newFake(), 100).Cleanup(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted, "only published rows older than 7 days")
}
