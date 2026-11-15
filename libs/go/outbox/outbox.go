// Package outbox implements the transactional outbox (spec 3, 5.3).
//
// A service writes the event into the outbox table in the SAME transaction
// as the data change (Write). A relay later reads unpublished rows,
// publishes them to Kafka and marks them published (Relay). There is no dual
// write: either both the change and the message are committed, or neither.
//
// Delivery is at-least-once: a crash between "published" and "marked" sends
// the message again. Consumers deduplicate by event_id (package inbox).
package outbox

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/sokolovlev/polka/libs/go/proto/polka/common/v1"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Schema holds the outbox and inbox DDL as a goose migration
// (00001_outbox_inbox.sql), for tests and as the reference that services
// copy into their first migration.
var Schema = mustSub(migrations, "migrations")

func mustSub(f fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err) // dir is a compile-time constant embedded above
	}
	return sub
}

// Message is one row of the outbox.
type Message struct {
	EventID uuid.UUID
	Topic   string
	// Kafka key = aggregate id: all events of one aggregate go to one
	// partition and are read in order (spec 6.3).
	Key string
	// Serialized commonv1.Envelope.
	Payload []byte
}

// Event describes a domain event to wrap into an Envelope.
type Event struct {
	Topic            string
	Type             string // "order.created"
	AggregateID      string
	AggregateVersion int64
	CorrelationID    string // order_id for saga messages
	Traceparent      string // W3C trace context, set in stage 4
	Payload          proto.Message
}

// NewMessage wraps ev into an Envelope with a fresh UUIDv7 event id.
func NewMessage(ev Event, now time.Time) (Message, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Message{}, fmt.Errorf("event id: %w", err)
	}
	payload, err := anypb.New(ev.Payload)
	if err != nil {
		return Message{}, fmt.Errorf("pack payload: %w", err)
	}
	env := &commonv1.Envelope{
		EventId:          id.String(),
		EventType:        ev.Type,
		AggregateId:      ev.AggregateID,
		AggregateVersion: ev.AggregateVersion,
		OccurredAt:       timestamppb.New(now),
		CorrelationId:    ev.CorrelationID,
		Traceparent:      ev.Traceparent,
		Payload:          payload,
	}
	b, err := proto.Marshal(env)
	if err != nil {
		return Message{}, fmt.Errorf("marshal envelope: %w", err)
	}
	return Message{EventID: id, Topic: ev.Topic, Key: ev.AggregateID, Payload: b}, nil
}

// Write inserts messages into the outbox within tx. Call it in the same
// transaction as the change the messages describe.
func Write(ctx context.Context, tx pgx.Tx, msgs ...Message) error {
	for _, m := range msgs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO outbox (event_id, topic, key, payload) VALUES ($1, $2, $3, $4)`,
			m.EventID, m.Topic, m.Key, m.Payload,
		); err != nil {
			return fmt.Errorf("insert outbox %s: %w", m.EventID, err)
		}
	}
	return nil
}
