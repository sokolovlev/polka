#!/usr/bin/env bash
# Verifies that `make infra` produced what spec 6.4, 7 and 7.4 require.
# Usage: deploy/compose/check-infra.sh   (or `make check-infra`)
set -uo pipefail
cd "$(dirname "$0")/../.." || exit 1
set -a
# shellcheck source=/dev/null
. ./.env
set +a
dc() { docker compose -f deploy/compose/compose.yaml --env-file .env --profile infra "$@"; }

fail=0
pass() { printf 'PASS %s\n' "$1"; }
bad()  { printf 'FAIL %s\n' "$1"; fail=1; }
check() { local name=$1; shift; if "$@" >/dev/null 2>&1; then pass "$name"; else bad "$name"; fi; }
check_not() { local name=$1; shift; if "$@" >/dev/null 2>&1; then bad "$name"; else pass "$name"; fi; }

# shellcheck disable=SC2317,SC2329 # called through check/check_not
psql_as() { # <user> <password> <db> <sql>
  dc exec -T -e PGPASSWORD="$2" postgres psql -h 127.0.0.1 -U "$1" -d "$3" -tAc "$4"
}

# --- PostgreSQL: database per service, no cross access (spec 7)
for svc in auth catalog orders inventory payments notifications; do
  var="POLKA_$(echo "$svc" | tr "[:lower:]" "[:upper:]")_DB_PASSWORD"
  check "postgres: $svc owns database $svc" psql_as "$svc" "${!var}" "$svc" "select 1"
done
check_not "postgres: auth cannot connect to catalog" psql_as auth "$POLKA_AUTH_DB_PASSWORD" catalog "select 1"
check_not "postgres: catalog cannot connect to payments" psql_as catalog "$POLKA_CATALOG_DB_PASSWORD" payments "select 1"
check "postgres: citext in auth" psql_as auth "$POLKA_AUTH_DB_PASSWORD" auth "select 'A'::citext = 'a'::citext"
check "postgres: ltree in catalog" psql_as catalog "$POLKA_CATALOG_DB_PASSWORD" catalog "select 'a.b'::ltree"

# --- Kafka topics (spec 6.4, R3)
describe=$(dc exec -T kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --describe 2>/dev/null)
for t in catalog.products.v1 catalog.shops.v1 inventory.stock.v1 inventory.commands.v1 inventory.replies.v1 \
         payment.commands.v1 payment.replies.v1 orders.events.v1 auth.users.v1; do
  check "kafka: $t has 6 partitions, RF 1" grep -qE "Topic: ${t}[[:space:]].*PartitionCount: 6[[:space:]].*ReplicationFactor: 1" <<<"$describe"
  check "kafka: $t.dlq exists" grep -qE "Topic: $t\.dlq[[:space:]]" <<<"$describe"
done
topic_cfg() { dc exec -T kafka /opt/kafka/bin/kafka-configs.sh --bootstrap-server localhost:9092 \
  --describe --entity-type topics --entity-name "$1" 2>/dev/null; }
broker_cfg() { dc exec -T kafka /opt/kafka/bin/kafka-configs.sh --bootstrap-server localhost:9092 \
  --describe --entity-type brokers --entity-name 1 --all 2>/dev/null; }
check "kafka: 18 topics (9 + DLQ)" \
  test "$(grep -c "PartitionCount" <<<"$describe")" -eq 18
for t in orders.events.v1 auth.users.v1.dlq; do
  check "kafka: $t min.insync.replicas=1" grep -q "min.insync.replicas=1 " <<<"$(topic_cfg "$t")"
done
check "kafka: auth.users.v1 retention.ms=86400000 (24 h)" grep -q "retention.ms=86400000 " <<<"$(topic_cfg auth.users.v1)"
check "kafka: auto topic creation is off" grep -q "auto.create.topics.enable=false " <<<"$(broker_cfg)"

# --- S3 (spec 7.4, R7)
s3() { dc run --rm -T --no-deps --entrypoint aws -e AWS_ACCESS_KEY_ID="$S3_ACCESS_KEY" -e AWS_SECRET_ACCESS_KEY="$S3_SECRET_KEY" -e AWS_DEFAULT_REGION=us-east-1 s3-init --endpoint-url http://s3:8333 "$@"; }
check "s3: bucket product-images exists" s3 s3api head-bucket --bucket product-images
echo thumb | s3 s3 cp - s3://product-images/thumbs/200/check.webp >/dev/null 2>&1
echo orig  | s3 s3 cp - s3://product-images/originals/check/check >/dev/null 2>&1
check "s3: thumbs/ readable anonymously" curl -fsS -o /dev/null http://127.0.0.1:8333/product-images/thumbs/200/check.webp
check_not "s3: originals/ not readable anonymously" curl -fsS -o /dev/null http://127.0.0.1:8333/product-images/originals/check/check
check_not "s3: anonymous write rejected" curl -fsS -o /dev/null -X PUT --data x http://127.0.0.1:8333/product-images/thumbs/200/evil.webp

# --- Elasticsearch, Valkey, Mailpit
check "elasticsearch: cluster yellow or green" curl -fsS "http://127.0.0.1:9200/_cluster/health?wait_for_status=yellow&timeout=5s"
check "elasticsearch: version 8" bash -c 'curl -fsS http://127.0.0.1:9200 | grep -q "\"number\" : \"8\."'
check "valkey: PING" bash -c 'docker compose -f deploy/compose/compose.yaml --env-file .env --profile infra exec -T valkey valkey-cli ping | grep -q PONG'
check "mailpit: UI up" curl -fsS -o /dev/null http://127.0.0.1:8025/

exit $fail
