# --- Стадия сборки ---
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY . .

# go mod tidy подтягивает зависимости и формирует go.sum,
# после чего собирается статический бинарник без CGO.
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
        -o /out/bank-api ./cmd/server

# --- Стадия запуска ---
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 appuser

WORKDIR /app

COPY --from=builder /out/bank-api /app/bank-api
COPY migrations /app/migrations

ENV APP_PORT=8080 \
    MIGRATIONS_DIR=/app/migrations

USER appuser
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/app/bank-api"]
