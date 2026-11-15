# Polka developer entry points. `make help` lists targets.
SHELL := /bin/bash
.DEFAULT_GOAL := help

BIN := $(CURDIR)/bin
export PATH := $(BIN):$(PATH)

# Pinned tool versions (ADR-0002). CI uses the same targets.
BUF_VERSION              := v1.75.0
PROTOC_GEN_GO_VERSION    := v1.36.12
PROTOC_GEN_GO_GRPC_VER   := v1.6.2
PROTOC_GEN_PROST_VERSION := 0.5.0
GOLANGCI_LINT_VERSION    := v2.15.0
OASDIFF_VERSION          := v1.37.0
GO_LICENSES_VERSION      := v2.0.1
CARGO_DENY_VERSION       := 0.20.2

GEN_DIRS := libs/go/proto libs/rust/proto/src/gen libs/ts/proto/src/gen

.PHONY: help
help: ## List targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-16s %s\n", $$1, $$2}'

# ---------------------------------------------------------------- tools
.PHONY: tools tools-go tools-rust tools-node
tools: tools-go tools-rust tools-node ## Install pinned code generators and linters into ./bin

tools-go:
	GOBIN=$(BIN) go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	GOBIN=$(BIN) go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	GOBIN=$(BIN) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VER)
	GOBIN=$(BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	GOBIN=$(BIN) go install github.com/oasdiff/oasdiff@$(OASDIFF_VERSION)

tools-rust:
	cargo install --locked --root $(CURDIR) \
	  protoc-gen-prost@$(PROTOC_GEN_PROST_VERSION) \
	  protoc-gen-tonic@$(PROTOC_GEN_PROST_VERSION) \
	  protoc-gen-prost-crate@$(PROTOC_GEN_PROST_VERSION)

tools-node:
	npm ci

# ---------------------------------------------------------------- proto
.PHONY: gen proto-lint proto-breaking gen-check
gen: ## Generate Go, Rust and TS code from proto/
	buf generate proto --template proto/buf.gen.yaml
	cargo fmt -p polka-proto

proto-lint: ## buf lint + format check
	buf lint proto
	buf format --diff --exit-code proto

proto-breaking: ## buf breaking against origin/main (skipped while main has no proto/)
	@if git cat-file -e origin/main:proto/buf.yaml 2>/dev/null; then \
	  buf breaking proto --against '.git#branch=origin/main,subdir=proto'; \
	else echo "origin/main has no proto/ yet, skipping buf breaking"; fi

gen-check: gen ## Fail if generated code differs from what is committed
	@if [ -n "$$(git status --porcelain -- $(GEN_DIRS))" ]; then \
	  git status --porcelain -- $(GEN_DIRS); \
	  echo "generated code is stale: run 'make gen' and commit"; exit 1; \
	fi

# ---------------------------------------------------------------- openapi
OPENAPI := services/gateway/api/openapi.yaml

.PHONY: openapi-lint openapi-breaking
openapi-lint: ## Lint the gateway OpenAPI spec (Redocly)
	npx redocly lint gateway

openapi-breaking: ## Fail on breaking OpenAPI changes against origin/main (oasdiff)
	@if git cat-file -e origin/main:$(OPENAPI) 2>/dev/null; then \
	  oasdiff breaking --fail-on ERR <(git show origin/main:$(OPENAPI)) $(OPENAPI); \
	else echo "origin/main has no $(OPENAPI) yet, skipping oasdiff"; fi

# ---------------------------------------------------------------- compose
# Build info is passed to compose only, so unit tests never see it.
POLKA_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
POLKA_COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
COMPOSE := POLKA_VERSION=$(POLKA_VERSION) POLKA_COMMIT=$(POLKA_COMMIT) \
  docker compose -f deploy/compose/compose.yaml --env-file .env

.env:
	cp .env.example .env

INFRA_SERVICES := postgres valkey kafka elasticsearch s3 mailpit
INIT_JOBS      := kafka-init s3-init
APP_SERVICES   := gateway auth catalog search order inventory payment payment-provider-mock notification web

.PHONY: up up-build infra down clean ps logs
# Images: ghcr.io/sokolovlev/polka-<service>:main, published by CI from main.
# Pulling them keeps `make up` within minutes on a clean machine (spec 10.3);
# compiling the Rust services alone can take longer than that.
up: infra ## Pull prebuilt images from GHCR and start everything (falls back to building)
	@$(COMPOSE) --profile apps pull $(APP_SERVICES) \
	  || { echo "pull failed: building images locally (make up-build)"; $(MAKE) --no-print-directory up-build; exit $$?; }
	$(COMPOSE) --profile apps up -d --no-build --wait $(APP_SERVICES)

up-build: infra ## Build images from source (tag :local) and start everything
	POLKA_TAG=local $(COMPOSE) --profile apps up -d --build --wait $(APP_SERVICES)

# `up --wait` treats a one-shot container that exits (even with 0) as a
# failure, so long-running services are awaited and init jobs are run with
# `run`, which returns the job's exit code. Init jobs are idempotent.
infra: .env ## Start infra (Postgres, Valkey, Kafka, ES, S3, Mailpit) and run init jobs
	$(COMPOSE) --profile infra up -d --wait $(INFRA_SERVICES)
	@for job in $(INIT_JOBS); do $(COMPOSE) --profile infra run --rm $$job || exit 1; done

down: .env ## Stop everything (keeps data volumes)
	$(COMPOSE) --profile infra --profile apps down

clean: .env ## Stop everything and delete data volumes
	$(COMPOSE) --profile infra --profile apps down --volumes

ps: .env ## Show containers and health
	$(COMPOSE) --profile infra --profile apps ps

logs: .env ## Follow logs (make logs S=order)
	$(COMPOSE) --profile infra --profile apps logs -f $(S)

# ---------------------------------------------------------------- checks
.PHONY: lint test lint-go test-go lint-rust test-rust lint-ts test-ts
GO_MODULES := libs/go services/gateway services/auth services/catalog services/search services/order

lint: proto-lint openapi-lint lint-go lint-rust lint-ts ## All linters
test: test-go test-rust test-ts ## All unit tests

lint-go:
	@for m in $(GO_MODULES); do echo "golangci-lint $$m"; (cd $$m && golangci-lint run ./...) || exit 1; done

test-go:
	@for m in $(GO_MODULES); do (cd $$m && go test -race ./...) || exit 1; done

lint-rust:
	cargo fmt --all --check
	cargo clippy --workspace --all-targets --locked -- -D warnings

test-rust:
	cargo test --workspace --locked

lint-ts:
	npm run build:libs
	npm run lint
	npm run typecheck
	npm run format:check

test-ts:
	npm run build:libs
	npm test

.PHONY: check-infra
check-infra: .env ## Verify databases, topics, bucket policy after `make infra`
	deploy/compose/check-infra.sh

.PHONY: secrets
secrets: ## Scan git history for leaked secrets (gitleaks, as in CI)
	GOBIN=$(BIN) go install github.com/zricethezav/gitleaks/v8@v8.30.1
	gitleaks git . --no-banner --redact

# ---------------------------------------------------------------- licenses
# R6: only free licenses. Allow-list: licenses.allow (deny.toml mirrors it).
.PHONY: licenses licenses-go licenses-rust licenses-npm
licenses: licenses-go licenses-rust licenses-npm ## Fail on dependencies with licenses outside licenses.allow

licenses-go:
	@test -x $(BIN)/go-licenses || GOBIN=$(BIN) go install github.com/google/go-licenses/v2@$(GO_LICENSES_VERSION)
	.github/scripts/licenses-go.sh $(GO_MODULES)

licenses-rust:
	@test -x $(BIN)/cargo-deny || cargo install --locked --root $(CURDIR) cargo-deny@$(CARGO_DENY_VERSION)
	cargo-deny --locked check licenses

licenses-npm:
	node .github/scripts/licenses-npm.mjs
