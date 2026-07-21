# syntax=docker/dockerfile:1

# ---- Build stage -----------------------------------------------------------
# Pin to the same Go line as go.mod (1.26.x). Alpine keeps the toolchain small;
# the binary itself is built fully static (CGO_ENABLED=0) so it does not depend
# on anything from this image at runtime.
FROM golang:1.26-alpine AS build

# git can be needed to resolve some module paths.
RUN apk add --no-cache git

WORKDIR /src

# Download dependencies first so this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Build the static binary. -trimpath + -s -w drop paths and debug info to keep
# the image lean and reproducible.
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/go-mcp-server ./cmd/server

# ---- Runtime stage ---------------------------------------------------------
# Alpine gives us ca-certificates (outbound TLS to OpenAI/GitHub/Kafka), tzdata
# for correct timestamps, and wget for the container HEALTHCHECK — all tiny.
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S app \
    && adduser -S -G app app

WORKDIR /app
COPY --from=build /out/go-mcp-server /usr/local/bin/go-mcp-server

# PORT is read by cmd/server/main.go (defaults to 8080 there too).
ENV PORT=8080
EXPOSE 8080

USER app

# /healthz is auth-exempt and served on the same port; treat a non-200 as down.
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -q -O /dev/null "http://127.0.0.1:${PORT}/healthz" || exit 1

ENTRYPOINT ["go-mcp-server"]
