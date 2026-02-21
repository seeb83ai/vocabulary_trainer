# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /vocab-trainer .

# Stage 2: Runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates python3 py3-pip && \
    pip3 install --break-system-packages --upgrade setuptools && \
    mkdir -p /data /opt/vocab-trainer/cmd/tts

COPY cmd/tts/requirements.txt /opt/vocab-trainer/cmd/tts/requirements.txt
RUN pip3 install --break-system-packages -r /opt/vocab-trainer/cmd/tts/requirements.txt
COPY cmd/tts/generate.py /opt/vocab-trainer/cmd/tts/generate.py

WORKDIR /app
COPY --from=builder /vocab-trainer /app/vocab-trainer

EXPOSE 8080

ENTRYPOINT ["/app/vocab-trainer"]
