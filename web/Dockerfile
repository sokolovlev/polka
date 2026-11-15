# syntax=docker/dockerfile:1
# Next.js in standalone mode (spec 9.1). Build from the repo root:
#   docker build -f web/Dockerfile .
ARG NODE_VERSION=22.23.5

FROM node:${NODE_VERSION}-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
COPY libs/ts/proto/package.json libs/ts/proto/
COPY libs/ts/runtime/package.json libs/ts/runtime/
COPY services/notification/package.json services/notification/
COPY web/package.json web/
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund -w @polka/web --include-workspace-root
COPY web web
ENV NEXT_TELEMETRY_DISABLED=1
RUN --mount=type=cache,target=/src/web/.next/cache npm run build -w @polka/web

FROM gcr.io/distroless/nodejs22-debian12:nonroot
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.title="polka-web" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.source="https://github.com/sokolovlev/polka"
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 HOSTNAME=0.0.0.0 PORT=3000 \
    POLKA_VERSION=${VERSION} POLKA_COMMIT=${COMMIT}
WORKDIR /app
# standalone keeps the workspace layout: the server is web/server.js.
COPY --from=build /src/web/.next/standalone ./
COPY --from=build /src/web/.next/static web/.next/static
COPY --from=build /src/web/public web/public
COPY --from=build /src/web/healthcheck.mjs web/healthcheck.mjs
USER nonroot:nonroot
EXPOSE 3000
CMD ["web/server.js"]
