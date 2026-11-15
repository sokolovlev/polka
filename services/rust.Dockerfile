# syntax=docker/dockerfile:1
# Shared Dockerfile for the Rust services (spec 9.1). Build from the repo root:
#   docker build -f services/rust.Dockerfile --build-arg SERVICE=inventory .
#
# cargo-chef splits the build in two: "cook" compiles only the dependencies
# from a recipe (Cargo.toml/Cargo.lock), so that layer is reused until a
# dependency changes; then the workspace code is compiled on top.
ARG RUST_VERSION=1.100.0

FROM rust:${RUST_VERSION}-slim-trixie AS chef
RUN cargo install cargo-chef --locked --version 0.1.78
WORKDIR /src

FROM chef AS planner
COPY Cargo.toml Cargo.lock rust-toolchain.toml clippy.toml ./
COPY libs/rust libs/rust
COPY services/inventory services/inventory
COPY services/payment services/payment
COPY services/payment-provider-mock services/payment-provider-mock
RUN cargo chef prepare --recipe-path recipe.json

FROM chef AS build
ARG SERVICE
COPY --from=planner /src/recipe.json recipe.json
COPY rust-toolchain.toml ./
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    cargo chef cook --release --locked -p ${SERVICE} --recipe-path recipe.json
COPY --from=planner /src .
ARG VERSION=dev
ARG COMMIT=unknown
# build_info! reads these at compile time.
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    POLKA_VERSION=${VERSION} POLKA_COMMIT=${COMMIT} \
    cargo build --release --locked -p ${SERVICE} \
 && cp target/release/${SERVICE} /service

# glibc + libgcc, no shell, non-root (8.3). debian12, not debian13: the
# debian13 cc base alone is ~29 MB unpacked, which leaves no room under the
# 30 MB target (9.1); debian12 is ~25 MB and supported until 2028.
FROM gcr.io/distroless/cc-debian12:nonroot
ARG SERVICE
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.title="polka-${SERVICE}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.source="https://github.com/sokolovlev/polka"
COPY --from=build /service /service
USER nonroot:nonroot
ENTRYPOINT ["/service"]
