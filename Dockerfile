# Stage 1: Build
FROM golang:1.25-alpine AS builder

WORKDIR /app/service

COPY service/go.mod service/go.sum ./
RUN go mod download

COPY service/ .

# Regenerate the landing page's feature-teaser grid from
# frontend/landing/teasers/*/teaser.json + images before the frontend is
# embedded into the binary, so the built image never ships a stale grid.
RUN go run ./cmd/gen-landing

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /vocab-trainer .

# Stage 2: Runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates && \
    mkdir -p /data

WORKDIR /app
COPY --from=builder /vocab-trainer /app/vocab-trainer

EXPOSE 8080

ENTRYPOINT ["/app/vocab-trainer"]
