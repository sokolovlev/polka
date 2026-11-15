# syntax=docker/dockerfile:1
# Dockerfile for the TS services (spec 9.1); today only notification.
# Build from the repo root:
#   docker build -f services/node.Dockerfile --build-arg SERVICE=notification .
ARG NODE_VERSION=22.23.5

FROM node:${NODE_VERSION}-alpine AS deps
ARG SERVICE
WORKDIR /src
# Every workspace manifest is needed for `npm ci` to match the lockfile.
COPY package.json package-lock.json ./
COPY libs/ts/proto/package.json libs/ts/proto/
COPY libs/ts/runtime/package.json libs/ts/runtime/
COPY services/notification/package.json services/notification/
COPY web/package.json web/

FROM deps AS build
ARG SERVICE
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund -w @polka/${SERVICE} -w @polka/proto -w @polka/runtime --include-workspace-root
COPY tsconfig.base.json ./
COPY libs/ts libs/ts
COPY services/${SERVICE} services/${SERVICE}
RUN npm run build -w @polka/proto -w @polka/runtime -w @polka/${SERVICE}

# Production dependencies only; workspace libs are symlinks in node_modules.
FROM deps AS prod-deps
ARG SERVICE
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund --omit=dev -w @polka/${SERVICE}

FROM gcr.io/distroless/nodejs22-debian12:nonroot
ARG SERVICE
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.title="polka-${SERVICE}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.source="https://github.com/sokolovlev/polka"
ENV NODE_ENV=production POLKA_VERSION=${VERSION} POLKA_COMMIT=${COMMIT}
WORKDIR /app
COPY --from=prod-deps /src/node_modules node_modules
COPY --from=build /src/libs/ts/proto/package.json libs/ts/proto/
COPY --from=build /src/libs/ts/proto/dist libs/ts/proto/dist
COPY --from=build /src/libs/ts/runtime/package.json libs/ts/runtime/
COPY --from=build /src/libs/ts/runtime/dist libs/ts/runtime/dist
COPY --from=build /src/services/${SERVICE}/package.json service/
COPY --from=build /src/services/${SERVICE}/dist service/dist
# node_modules/@polka/* link to ../../libs/ts/*; the service itself is not
# linked from node_modules, so it can live in /app/service.
USER nonroot:nonroot
# distroless/nodejs's entrypoint is node.
CMD ["service/dist/main.js"]
