# Stage 1: Build
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /vocab-trainer .

# Stage 2: Runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates && \
    mkdir -p /data && \
    adduser -D -H -u 1001 appuser && \
    chown appuser /data

WORKDIR /app
COPY --from=builder /vocab-trainer /app/vocab-trainer

EXPOSE 8080

USER appuser

ENTRYPOINT ["/app/vocab-trainer"]
