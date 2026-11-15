# 0005. Outbox relay with an advisory lock; inbox in the same transaction

- Status: Accepted
- Date: 2026-11-14
- Spec: 3, 5.3, 6.3, 6.4, 7.1, R28

## Context

Every change other services must learn about is published through a
transactional outbox, and every consumer is idempotent (spec 3). Spec 5.3:
the relay reads batches of 100 with `FOR UPDATE SKIP LOCKED` "so several
replicas do not get in each other's way", publishes and sets `published_at`;
consumers record `event_id` in `processed_messages` in the same transaction
as the change. Messages of one aggregate share a Kafka key, so they land in
one partition and are read in order.

`SKIP LOCKED` alone breaks that order. Replica A locks rows 1–100, replica B
skips them and takes 101–200, and both publish at the same time. If order X
has events at ids 50 and 150, B can deliver 150 before A delivers 50: same
partition, wrong order.

## Options

1. **SKIP LOCKED only** (spec text). Parallel relays, no ordering guarantee
   across batches; every consumer must tolerate reordering.
2. **Single active relay via `pg_try_advisory_xact_lock`** taken at the start
   of each batch transaction. Only one replica publishes at a time, others
   return immediately and retry later; the lock is released with the
   transaction, so a crashed replica never blocks the others.
3. **Session advisory lock as leader election.** Same effect, but the leader
   must hold a dedicated connection and detect losing it.
4. **Partition the outbox by key hash**, one relay per partition. Parallel
   and ordered, much more machinery.

## Decision

Option 2, keeping `FOR UPDATE SKIP LOCKED` as a second guard.

Per batch, in one transaction:

1. `SELECT pg_try_advisory_xact_lock(<service key>)`; if false, return.
2. `SELECT … WHERE published_at IS NULL ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`.
3. Publish in id order and wait for acks (`acks=all`, idempotent producer:
   order is kept per partition across the producer's own retries).
4. Mark `published_at` only for the acknowledged prefix, then commit.

Inbox (`inbox.Handle`): in one transaction
`INSERT INTO processed_messages … ON CONFLICT DO NOTHING`; zero rows means a
duplicate, so skip; otherwise run the handler. A concurrent duplicate blocks
on the unique index until the first transaction commits, then sees the
conflict, so exactly one applies the change.

Kafka client: franz-go (BSD-3, pure Go; no cgo, so distroless/static still
works). Integration tests run Kafka from `apache/kafka`, not the
testcontainers module image `confluent-local`, which is under the Confluent
Community License (R6).

## Consequences

- Global order per service outbox is kept, which implies order per key.
- Throughput is one relay per service. For a laptop demo it is far above what
  is needed (100 messages per round trip to Kafka).
- The batch transaction stays open while Kafka acknowledges (bounded by the
  delivery timeout, 10 s). Rows are locked only for the relay itself; writers
  insert new rows and are not blocked.
- A crash after Kafka acked but before commit republishes the batch:
  at-least-once. Duplicates are absorbed by the inbox (event_id) and, for
  search, by external versioning.
- Republishing from the first failed message can put an already delivered
  later message of the same key before it (only after a failure). Saga
  handlers ignore replies that do not fit the current step (5.3), so this is
  safe.
