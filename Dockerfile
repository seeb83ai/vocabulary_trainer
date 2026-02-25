# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /vocab-trainer .

# Stage 2: Runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates && \
    mkdir -p /data

WORKDIR /app
COPY --from=builder /vocab-trainer /app/vocab-trainer

EXPOSE 8080

ENTRYPOINT ["/app/vocab-trainer"]
