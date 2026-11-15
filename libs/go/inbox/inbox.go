// Package inbox makes consumers idempotent (spec 3, 5.3): a message is
// applied at most once per event_id, even though Kafka delivers it at least
// once.
//
// The dedup record and the data change live in the SAME transaction: if the
// change fails, the record is rolled back too, and the redelivered message
// is processed again.
package inbox

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sokolovlev/polka/libs/go/pg"
)

// Handle runs fn exactly once for eventID. It returns processed=false
// (and does not call fn) when eventID was already processed.
//
// The INSERT ... ON CONFLICT DO NOTHING and fn share one transaction:
//   - a duplicate inserts 0 rows and is skipped, without an error;
//   - if fn fails, the dedup row is rolled back with fn's changes, so the
//     redelivered message is processed again;
//   - a concurrent duplicate blocks on the primary key until the first
//     transaction commits, then sees the conflict: exactly one applies fn.
func Handle(ctx context.Context, pool *pgxpool.Pool, eventID uuid.UUID,
	fn func(ctx context.Context, tx pgx.Tx) error,
) (processed bool, err error) {
	err = pg.InTx(ctx, pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO processed_messages (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, eventID)
		if err != nil {
			return fmt.Errorf("inbox insert: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		if err := fn(ctx, tx); err != nil {
			return err
		}
		processed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return processed, nil
}
