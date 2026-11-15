-- Shared tables of every database that publishes or consumes events
-- (spec 7.1). Services copy this into their first goose migration.

-- +goose Up
-- transactional outbox: written in the same transaction as the data change
CREATE TABLE outbox (
  id bigserial PRIMARY KEY,
  event_id uuid UNIQUE NOT NULL,
  topic text NOT NULL,
  key text NOT NULL,
  payload bytea NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  published_at timestamptz
);
-- the relay reads only unpublished rows, in id order
CREATE INDEX outbox_unpublished ON outbox (id) WHERE published_at IS NULL;

-- inbox: deduplication of incoming messages
CREATE TABLE processed_messages (
  event_id uuid PRIMARY KEY,
  processed_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE processed_messages;
DROP TABLE outbox;
