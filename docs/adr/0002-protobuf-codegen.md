# 0002. Protobuf code generation: local plugins, committed output

- Status: Accepted
- Date: 2026-11-14
- Spec: 6, 6.1, 6.3, 9.5, R1

## Context

All contracts are protobuf in `proto/`, generated for Go, Rust and TS with
`buf generate` (6). CI must check that the generated code is up to date (9.5).
Open choices: where plugins come from, where the output goes, and whether the
output is committed.

## Options

1. **Remote plugins on buf.build (BSR).** No local install. But every
   generation needs the network and is rate limited for anonymous users, and
   the plugin version lives on a third-party service.
2. **Local plugins pinned in the Makefile.** `make tools` installs exact
   versions into `./bin` (`go install`, `cargo install --locked`, `npm ci`).
   Works offline after the first install; versions are reviewed in git.
3. **Rust codegen in `build.rs` (tonic-build)** instead of buf. Idiomatic for
   Rust, but then Rust uses a different generator configuration from Go and TS,
   and needs `protoc` at build time inside Docker.

## Decision

Option 2, with the generated code committed.

| Language | Plugins | Output |
|---|---|---|
| Go | protoc-gen-go v1.36.12, protoc-gen-go-grpc v1.6.2 | `libs/go/proto` (module `libs/go`) |
| Rust | protoc-gen-prost, protoc-gen-tonic, protoc-gen-prost-crate 0.5.0 | `libs/rust/proto/src/gen` (crate `polka-proto`) |
| TS | ts-proto 2.21.1 (messages only, `forceLong=bigint`) | `libs/ts/proto/src/gen` (package `@polka/proto`) |

- Committed output: a Docker build or a reviewer needs no generator, and the
  diff of generated code is visible in PRs. CI runs `make gen-check`: it
  regenerates and fails on any difference.
- `buf breaking` runs against `origin/main` (FILE rules) and is skipped while
  main has no `proto/` yet.
- Stage 0 has only common types (`Money`, `PageRequest`/`PageResponse`,
  `Envelope`) and empty services (R1). Methods arrive with the stage that
  needs them; adding fields, messages and RPCs is backward compatible.
- int64 is `bigint` in TS: money is int64 kopecks, and `number` would silently
  lose precision above 2^53.
- `Timestamp` stays an object in TS (`useDate=false`): `Date` keeps only
  milliseconds and would change the bytes on re-encode.

**Cross-language golden test.** `proto/testdata/envelope_money.hex` is one
`Envelope` with a `Money` in `Any`. Go, Rust and TS tests each build the same
message, compare their encoding byte for byte and decode the fixture back.

The test found a real incompatibility. protoc-gen-prost has no option for the
`Any` type URL domain, so prost's `Any::from_msg` writes `/polka.common.v1.Money`,
while Go and TS write `type.googleapis.com/polka.common.v1.Money`. Fix:
`polka_proto::pack()` sets the Go/TS prefix, and `clippy.toml` forbids
`Any::from_msg` via `disallowed-methods`. Decoding is unaffected, because prost
compares only the full name.

## Consequences

- `make tools` takes a few minutes the first time (the Rust plugins compile).
  CI caches `./bin`.
- Changing a plugin version means `make tools && make gen` and committing the
  diff; the golden test catches wire-format changes.
- `buf generate` cleans its output directories (`clean: true`), so hand-written
  code never goes there. The Go golden test lives in `libs/go/internal/protogolden`.
