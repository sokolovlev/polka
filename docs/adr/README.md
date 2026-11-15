# Architecture Decision Records

Short records of decisions that are not obvious from the spec (`docs/spec.md`).
One file per decision: `NNNN-short-title.md`, numbered in order, never renumbered.
A superseded ADR is not edited; a new one references it and its status becomes
`Superseded by NNNN`.

Start from [`template.md`](template.md).

| # | Title | Status |
|---|---|---|
| [0001](0001-monorepo-layout-and-toolchains.md) | Monorepo layout, build units and toolchains | Accepted |
| [0002](0002-protobuf-codegen.md) | Protobuf code generation: local plugins, committed output | Accepted |
| [0003](0003-s3-storage-seaweedfs.md) | S3 storage: SeaweedFS instead of MinIO | Accepted |
| [0004](0004-free-licenses-only.md) | Free licenses only, checked in CI | Accepted |
| [0005](0005-outbox-relay-and-inbox.md) | Outbox relay with an advisory lock; inbox in the same transaction | Accepted |
