# Polka

A learning multi-vendor marketplace: Go, Rust and TypeScript microservices,
an orchestrated order saga over Kafka, transactional outbox/inbox,
double-entry ledger, Kubernetes.

The specification is in [`docs/spec.md`](docs/spec.md) (scope decisions for a
portfolio prototype in [`docs/spec-questions.md`](docs/spec-questions.md)),
architecture decisions in [`docs/adr/`](docs/adr/).

## Status

| Stage (spec 10.3) | State |
|---|---|
| 0. Skeleton: monorepo, proto + codegen for Go/Rust/TS, services with health checks, compose, CI | done |
| 1. Storefront: shared Go platform (Postgres, Valkey, gRPC client, errors), outbox/inbox, Kafka consumer | foundation done |
| 1. Storefront: auth, catalog, gateway handlers, web pages | not started |
| 2. Search, 3. Orders (saga, inventory, payment, ledger), 4. Notifications and tracing, 5. Kubernetes | not started |

What can be shown today:

- `make up`: 16 containers (Postgres, Valkey, Kafka KRaft, Elasticsearch,
  SeaweedFS S3, Mailpit and 10 service skeletons) start healthy;
  `make check-infra` verifies databases, topics and bucket policies.
- The same protobuf `Envelope` encodes to identical bytes in Go, Rust and TS
  (`proto/testdata`), which found a real `Any` type URL mismatch (ADR-0002).
- Transactional outbox with an ordering-safe relay, idempotent inbox and a
  consumer with retries and DLQ, tested against real Postgres and Kafka
  (`libs/go/{outbox,inbox,consumer}`, ADR-0005).
- Only free licenses, enforced in CI (`make licenses`, ADR-0004).

## Quick start

Requirements: Docker Engine (or Docker Desktop) with Compose v2, GNU make.

```bash
git clone https://github.com/sokolovlev/polka && cd polka
make up          # pulls prebuilt images, starts infra + services, waits until healthy
make check-infra # verifies databases, Kafka topics and the S3 bucket
make down        # stop (make clean also deletes data volumes)
```

| What | URL |
|---|---|
| Gateway | http://localhost:8080/version |
| Web | http://localhost:3000 |
| Mailpit (emails) | http://localhost:8025 |
| Elasticsearch | http://localhost:9200 |
| S3 (SeaweedFS) | http://localhost:8333 |

`make up` pulls images published by CI from main and builds locally if the pull
fails; `make up-build` always builds from source. `make help` lists all targets.

## Development

Toolchains: Go 1.27, Rust 1.100 (pinned in `rust-toolchain.toml`), Node.js 22.

```bash
make tools   # pinned buf plugins, golangci-lint, oasdiff into ./bin
npm ci
make gen     # regenerate protobuf code (committed; CI checks it is current)
make lint
make test
make licenses   # only free licenses (ADR-0004)
```

| Path | Contents |
|---|---|
| `proto/` | protobuf contracts, buf config |
| `services/` | gateway, auth, catalog, search, order (Go); inventory, payment, payment-provider-mock (Rust); notification (TS) |
| `web/` | Next.js 15 |
| `libs/{go,rust,ts}/` | generated protobuf code and the shared service runtime |
| `deploy/compose/` | docker compose and init scripts |
| `services/gateway/api/openapi.yaml` | public REST API (OpenAPI 3.1) |

## License

MIT, see [`LICENSE`](LICENSE).
