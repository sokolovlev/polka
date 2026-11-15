package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sokolovlev/polka/libs/go/pg"
)

// Publisher sends messages to the broker.
type Publisher interface {
	// Publish sends msgs in order and waits for acks (acks=all). It returns
	// how many LEADING messages were acknowledged: msgs[:n] are delivered,
	// msgs[n:] may or may not be. On full success n == len(msgs), err == nil.
	Publish(ctx context.Context, msgs []Message) (n int, err error)
}

// Relay moves messages from the outbox table to Kafka. Run one per service
// replica; replicas coordinate through a Postgres advisory lock (ADR-0005).
type Relay struct {
	Pool      *pgxpool.Pool
	Publisher Publisher
	Log       *slog.Logger
	// LockKey identifies this service's relay in pg_try_advisory_xact_lock.
	// Each service has its own database, so any constant works.
	LockKey int64
	// BatchSize rows per transaction (spec 5.3: 100).
	BatchSize int
	// Interval between polls when the outbox is empty.
	Interval time.Duration
	// Retention of published rows (spec 5.3: 7 days).
	Retention time.Duration

	once sync.Once
	stop chan struct{}
	done chan struct{}
}

// Defaults used when the fields are zero.
const (
	DefaultBatchSize = 100
	DefaultInterval  = 200 * time.Millisecond
	DefaultRetention = 7 * 24 * time.Hour
	cleanupEvery     = time.Hour
)

// publishBatch publishes at most BatchSize unpublished messages, oldest
// first, and marks the delivered ones published. It returns how many it
// marked. One transaction per batch (ADR-0005):
//   - pg_try_advisory_xact_lock(LockKey): only one replica publishes at a
//     time, so events keep their order; if taken, return 0, nil;
//   - rows WHERE published_at IS NULL ORDER BY id, FOR UPDATE SKIP LOCKED;
//   - publish in id order, mark only the acknowledged prefix, commit;
//   - Publish's error is returned after the commit, so the prefix stays
//     marked and the next batch starts at the first failed message.
func (r *Relay) publishBatch(ctx context.Context) (int, error) {
	var marked int
	var pubErr error
	err := pg.InTx(ctx, r.Pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, r.LockKey).Scan(&locked); err != nil {
			return fmt.Errorf("advisory lock: %w", err)
		}
		if !locked {
			return nil
		}
		rows, err := tx.Query(ctx, `SELECT id, event_id, topic, key, payload FROM outbox
			WHERE published_at IS NULL ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED`, r.batchSize())
		if err != nil {
			return fmt.Errorf("select outbox: %w", err)
		}
		var ids []int64
		var msgs []Message
		for rows.Next() {
			var id int64
			var m Message
			if err := rows.Scan(&id, &m.EventID, &m.Topic, &m.Key, &m.Payload); err != nil {
				rows.Close()
				return fmt.Errorf("scan outbox: %w", err)
			}
			ids = append(ids, id)
			msgs = append(msgs, m)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("read outbox: %w", err)
		}
		if len(msgs) == 0 {
			return nil
		}
		n, err := r.Publisher.Publish(ctx, msgs)
		pubErr = err
		if n == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = ANY($1)`, ids[:n]); err != nil {
			return fmt.Errorf("mark published: %w", err)
		}
		marked = n
		return nil
	})
	if err != nil {
		return 0, err
	}
	if pubErr != nil {
		return marked, fmt.Errorf("publish: %w", pubErr)
	}
	return marked, nil
}

// Cleanup deletes published rows older than Retention.
func (r *Relay) Cleanup(ctx context.Context) (int64, error) {
	tag, err := r.Pool.Exec(ctx,
		`DELETE FROM outbox WHERE published_at IS NOT NULL AND published_at < $1`,
		time.Now().Add(-r.retention()))
	if err != nil {
		return 0, fmt.Errorf("outbox cleanup: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Name implements lifecycle.Component.
func (r *Relay) Name() string { return "outbox-relay" }

// Run polls until Shutdown. A full batch is followed immediately by the next
// one; an empty or partial batch waits Interval. Errors are logged and
// retried: the relay must outlive a Kafka outage.
func (r *Relay) Run(ctx context.Context) error {
	r.init()
	defer close(r.done)
	lastCleanup := time.Time{}
	for {
		n, err := r.publishBatch(ctx)
		if err != nil {
			r.Log.Warn("outbox publish failed", slog.Any("error", err), slog.Int("published", n))
		}
		if time.Since(lastCleanup) > cleanupEvery {
			if deleted, err := r.Cleanup(ctx); err != nil {
				r.Log.Warn("outbox cleanup failed", slog.Any("error", err))
			} else if deleted > 0 {
				r.Log.Info("outbox cleanup", slog.Int64("deleted", deleted))
			}
			lastCleanup = time.Now()
		}
		if n == r.batchSize() && err == nil {
			select {
			case <-r.stop:
				return nil
			default:
				continue
			}
		}
		select {
		case <-r.stop:
			return nil
		case <-time.After(r.interval()):
		}
	}
}

// Shutdown stops polling after the current batch.
func (r *Relay) Shutdown(ctx context.Context) error {
	r.init()
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("outbox relay: %w", ctx.Err())
	}
}

func (r *Relay) init() {
	r.once.Do(func() {
		r.stop = make(chan struct{})
		r.done = make(chan struct{})
	})
}

func (r *Relay) batchSize() int {
	if r.BatchSize > 0 {
		return r.BatchSize
	}
	return DefaultBatchSize
}

func (r *Relay) interval() time.Duration {
	if r.Interval > 0 {
		return r.Interval
	}
	return DefaultInterval
}

func (r *Relay) retention() time.Duration {
	if r.Retention > 0 {
		return r.Retention
	}
	return DefaultRetention
}
