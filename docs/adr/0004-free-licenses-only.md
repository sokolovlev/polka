# 0004. Free licenses only, checked in CI

- Status: Accepted
- Date: 2026-11-14
- Spec: 1, R6

## Context

Polka is a portfolio prototype. Everything it uses must be free to use
(R6): no commercial or source-available-only licenses, no paid tiers. Two
components of the original spec already failed this rule: Redis 7.4+
(RSALv2/SSPL, replaced by Valkey, R9) and MinIO (community edition
discontinued, replaced by SeaweedFS, R7). A rule kept only in memory will
fail again as dependencies change.

## Options

1. **Review licenses by hand** when adding a dependency. Misses transitive
   dependencies (a Next.js install pulls in ~400 packages).
2. **Automated check against an allow-list** in every ecosystem, in CI.

## Decision

Option 2. `licenses.allow` lists the allowed SPDX ids: permissive licenses,
plus MPL-2.0 and LGPL-3.0 (free copyleft, used only by optional Next.js
packages: sharp/libvips, lightningcss), plus data licenses of browser tooling
(CC-BY-4.0 for caniuse-lite, CC0-1.0). `make licenses` and the CI job
`licenses` check:

| Ecosystem | Tool | Result now |
|---|---|---|
| Go | go-licenses v2 | all modules: Apache-2.0, BSD-3-Clause, MIT |
| Rust | cargo-deny (`deny.toml` mirrors the list) | 97 crates, mostly MIT / Apache-2.0 |
| npm | `.github/scripts/licenses-npm.mjs` over `package-lock.json` | 485 packages |

Each check was confirmed to fail when MIT is removed from the list.

Components that are not package dependencies are listed here and reviewed
when they change:

| Component | License | Note |
|---|---|---|
| PostgreSQL 16 | PostgreSQL License | |
| Valkey 8 | BSD-3-Clause | instead of Redis (R9) |
| Apache Kafka 4 | Apache-2.0 | |
| Elasticsearch 8 | Elastic License 2.0 / SSPL / AGPL-3.0 (choice) | free for self-managed use (R2) |
| SeaweedFS | Apache-2.0 | instead of MinIO (R7) |
| Mailpit | MIT | |
| AWS CLI (s3-init) | Apache-2.0 | |
| distroless base images | Apache-2.0 (Debian packages: free licenses) | |
| Docker Engine, Compose | Apache-2.0 | Docker Desktop is free for personal use and small companies; Docker Engine is enough |
| GitHub Actions, GHCR | free for public repositories | |
| Dev tools: buf, oasdiff, Trivy, go-licenses | Apache-2.0 | |
| Dev tools: gitleaks, Redocly CLI, actionlint | MIT | |
| Dev tools: cargo-deny, cargo-chef | MIT / Apache-2.0 | |
| Dev tools: golangci-lint, shellcheck | GPL-3.0 | free; used as tools, not shipped |

Planned for later stages (to be re-checked when added):

| Component | License | Note |
|---|---|---|
| OpenTelemetry, Jaeger, Prometheus | Apache-2.0 | stage 4 (R12) |
| Grafana | AGPL-3.0 | free; run unmodified |
| k6 | AGPL-3.0 | free |
| kind, Helm | Apache-2.0 | stage 5 (R11) |
| testcontainers, Playwright | MIT, Apache-2.0 | |
| kafkajs | MIT | unmaintained since 2023 (license is fine); compare with Confluent's `@confluentinc/kafka-javascript` (MIT) in stage 4 |
| ingress-nginx | Apache-2.0 | as far as I know the project is being retired; pick an alternative in stage 5 (for example Traefik, MIT) |

## Consequences

- A new dependency with a non-allowed license fails CI with its name and
  license; adding a license to the list is a visible, reviewed change.
- Components outside package managers (images, services, tools) still rely
  on the table above being updated when they change.
- Copyleft is allowed only for free licenses; source-available licenses
  (BUSL, SSPL-only, RSAL, Elastic License-only, Commons Clause) are not.
