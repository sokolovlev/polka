# syntax=docker/dockerfile:1
# Shared Dockerfile for the Go services (spec 9.1). Build from the repo root:
#   docker build -f services/go.Dockerfile --build-arg SERVICE=auth .
ARG GO_VERSION=1.27.4

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
ARG SERVICE
ARG VERSION=dev
ARG COMMIT=unknown
ARG TARGETOS
ARG TARGETARCH
ENV GOWORK=off CGO_ENABLED=0 GOTOOLCHAIN=local

# Dependencies first: this layer is reused until a go.mod/go.sum changes.
WORKDIR /src
COPY libs/go/go.mod libs/go/go.sum libs/go/
COPY services/${SERVICE}/go.mod services/${SERVICE}/go.sum services/${SERVICE}/
WORKDIR /src/services/${SERVICE}
RUN --mount=type=cache,target=/go/pkg/mod go mod download

WORKDIR /src
COPY libs/go libs/go
COPY services/${SERVICE} services/${SERVICE}
WORKDIR /src/services/${SERVICE}
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags "-s -w \
        -X github.com/sokolovlev/polka/libs/go/buildinfo.Version=${VERSION} \
        -X github.com/sokolovlev/polka/libs/go/buildinfo.Commit=${COMMIT}" \
      -o /out/service ./cmd/${SERVICE}

# Static binary on distroless: no shell, no package manager, non-root (8.3).
FROM gcr.io/distroless/static-debian13:nonroot
ARG SERVICE
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.title="polka-${SERVICE}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.source="https://github.com/sokolovlev/polka"
COPY --from=build /out/service /service
USER nonroot:nonroot
ENTRYPOINT ["/service"]
