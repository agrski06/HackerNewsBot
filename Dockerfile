# ---- Build stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bot ./cmd/bot

# ---- Run stage ----
FROM alpine:3

RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=builder /bot .

# Data directory for BoltDB
RUN mkdir -p /data
ENV HNB_DB_PATH=/data/seen.db

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -f http://localhost:8080/healthz || exit 1

ENTRYPOINT ["./bot"]

