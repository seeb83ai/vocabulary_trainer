.PHONY: build run stop logs dev tidy clean import release test test-go test-js tts-setup

# Load .env if present (for RSYNC_DEST)
-include .env
export

## build: build the Docker image
build:
	docker compose build

## run: start the app in the background
run:
	mkdir -p data
	docker compose up -d

## stop: stop the running container
stop:
	docker compose down

## logs: tail container logs
logs:
	docker compose logs -f

## dev: run locally without Docker (requires Go 1.22+)
dev:
	mkdir -p data
	DB_PATH=data/vocab.db go run .

## tidy: tidy Go module dependencies
tidy:
	go mod tidy

## import: import vocabulary from a text file (FILE=voc.txt DB=data/vocab.db)
import:
	mkdir -p data
	go run ./cmd/import -db $(or $(DB),data/vocab.db) -file $(or $(FILE),voc.txt)

## release: cross-compile for Raspberry Pi (arm64) and rsync to RSYNC_DEST
release:
	@test -n "$(RSYNC_DEST)" || (echo "RSYNC_DEST is not set. Copy .env.example to .env and fill it in." && exit 1)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s" -o vocab-trainer .
	rsync -avz --progress \
		vocab-trainer \
		deploy/vocab-trainer.service \
		deploy/nginx.conf \
		$(RSYNC_DEST)/
	@echo ""
	@echo "Done. On the Pi, run once to install the service:"
	@echo "  sudo cp /opt/vocab-trainer/vocab-trainer.service /etc/systemd/system/vocab-trainer.service"
	@echo "  sudo systemctl daemon-reload && sudo systemctl enable --now vocab-trainer"
	@echo ""
	@echo "To install nginx config:"
	@echo "  sudo cp /opt/vocab-trainer/nginx.conf /etc/nginx/sites-available/vocab-trainer"
	@echo "  sudo ln -sf /etc/nginx/sites-available/vocab-trainer /etc/nginx/sites-enabled/vocab-trainer"
	@echo "  sudo nginx -t && sudo systemctl reload nginx"

## test: run all tests (Go + JS)
test: test-go test-js

## test-go: run Go tests (uses in-memory SQLite, no server needed)
test-go:
	go test ./... -count=1

## test-js: run frontend tests with Vitest (requires Node; run 'npm install' first)
test-js:
	npm test

## tts-setup: create Python venv and install edge-tts (run once on host or Pi)
tts-setup:
	python3 -m venv tts-venv
	tts-venv/bin/pip install --upgrade pip setuptools
	tts-venv/bin/pip install -r cmd/tts/requirements.txt
	@echo ""
	@echo "Venv ready. Set these env vars to enable TTS:"
	@echo "  TTS_SCRIPT=$(CURDIR)/cmd/tts/generate.py"
	@echo "  VENV_PYTHON=$(CURDIR)/tts-venv/bin/python3"
	@echo "  AUDIO_DIR=data/audio  (or wherever your data lives)"

## clean: stop containers and remove build artifacts
clean:
	docker compose down --rmi local --volumes
	rm -f vocab-trainer
