# 0001. Monorepo layout, build units and toolchains

- Status: Accepted
- Date: 2026-11-14
- Spec: 1, 9.1, 9.5, 10.2

## Context

The spec fixes a single repository with `services/`, `libs/{go,rust,ts}`,
`proto/`, `web/` (10.2). It does not say how the three languages are split into
build units. The split decides three things we care about:

- whether a service can be built and versioned on its own (CI paths filter, 9.5);
- how a Docker build gets shared code without copying the whole repo (9.1);
- how many lock files and dependency trees we maintain.

## Options

1. **One build unit per language** (one Go module, one Cargo package, one npm
   package). Simplest, but every service shares one dependency graph and one
   `go.mod`; a dependency bump in one service rebuilds and retests all of them.
2. **Fully independent units** (each service with its own copy of shared code
   or a published library). Real isolation, but publishing internal libraries
   is overhead with no value for a single-team repo.
3. **Workspace per language, unit per service**: each service is its own Go
   module / crate / npm package, shared code lives in `libs/*`, and the
   language's workspace tool links them locally.

## Decision

Option 3.

- **Go.** Each service under `services/<name>` is its own module
  (`github.com/sokolovlev/polka/services/<name>`). Shared code is one module,
  `github.com/sokolovlev/polka/libs/go`, which also holds the generated
  protobuf code (`libs/go/proto`). Services depend on it through a
  `replace => ../../libs/go` directive, so `GOWORK=off go build` works inside
  a Docker build. `go.work` at the root is for editors and `make` targets.
- **Rust.** One Cargo workspace at the repo root with `libs/rust/*` and the
  three Rust services as members. One `Cargo.lock`, so all Rust services use
  the same versions of tokio, tonic, sqlx. Generated code is the crate
  `polka-proto` in `libs/rust/proto`.
- **TypeScript.** npm workspaces at the root: `libs/ts/*`,
  `services/notification`, `web`. Generated code is the package `@polka/proto`
  in `libs/ts/proto`. npm, not pnpm: it ships with Node, one less tool for a
  reviewer.

Docker builds use the repo root as context and copy only the service, the
libraries of its language and the lock files.

Toolchains (R5: current stable for Go and Rust, Next.js stays on 15):

| Tool | Version | Pinned in |
|---|---|---|
| Go | 1.27 | `go` directive in every `go.mod` (GOTOOLCHAIN downloads it) |
| Rust | 1.100.0 | `rust-toolchain.toml` |
| Node.js | 22 LTS | `.nvmrc`, `engines` in `package.json` |
| Next.js | 15.5 | `web/package.json` |
| TypeScript | 5.9 | root `package.json` (7.x drops the JS compiler API that typescript-eslint and Next 15 use) |
| ESLint | 9 | root `package.json` (eslint-config-next 15 does not support 10) |

## Consequences

- CI can build and test one service by its path (`services/order/**`) plus the
  libraries it depends on.
- A change in `libs/go` triggers every Go service; that is correct, since they
  all compile against it.
- Generated code is a library in each language, not part of `proto/`: `proto/`
  stays language-neutral, and services import generated types like any other
  dependency.
- Go modules do not share a lock: two Go services may resolve different minor
  versions of the same dependency. Acceptable; `go.work` keeps local
  development consistent.
