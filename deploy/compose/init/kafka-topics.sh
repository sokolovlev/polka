#!/bin/sh
# Creates the Kafka topics from spec 6.4. Idempotent: safe to rerun.
# docker compose: RF=1 and min.insync.replicas=1 (R3); Kubernetes uses RF=3, 2.
set -eu

BOOTSTRAP="${KAFKA_BOOTSTRAP:-kafka:9092}"
PARTITIONS="${KAFKA_PARTITIONS:-6}"
RF="${KAFKA_REPLICATION_FACTOR:-1}"
MIN_ISR="${KAFKA_MIN_INSYNC_REPLICAS:-1}"
BIN=/opt/kafka/bin

topic() { # <name> [extra configs, comma-separated k=v]
  name="$1"
  cfg="min.insync.replicas=$MIN_ISR${2:+,$2}"
  "$BIN/kafka-topics.sh" --bootstrap-server "$BOOTSTRAP" --create --if-not-exists \
    --topic "$name" --partitions "$PARTITIONS" --replication-factor "$RF" 2>&1 \
    | grep -v "could collide" || true
  # Applied separately so a rerun also updates an existing topic.
  "$BIN/kafka-configs.sh" --bootstrap-server "$BOOTSTRAP" --alter \
    --entity-type topics --entity-name "$name" --add-config "$cfg" >/dev/null
}

# Consumed topics get a <topic>.dlq for messages that failed 5 times (5.3).
with_dlq() {
  topic "$1" "${2:-}"
  topic "$1.dlq"
}

with_dlq catalog.products.v1
with_dlq catalog.shops.v1
with_dlq inventory.stock.v1
with_dlq inventory.commands.v1
with_dlq inventory.replies.v1
with_dlq payment.commands.v1
with_dlq payment.replies.v1
with_dlq orders.events.v1
# One-time tokens travel in this topic: keep at most 24 h (6.4, see Q4).
with_dlq auth.users.v1 retention.ms=86400000

"$BIN/kafka-topics.sh" --bootstrap-server "$BOOTSTRAP" --list
