.PHONY: build run stop logs dev tidy clean import import-hsk backup release test test-go test-js

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

start: run

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

## import-hsk: fetch and import HSK vocabulary from mandarinbean.com (LEVELS=1,2,3,4,5,6 DB=data/vocab.db)
import-hsk:
	mkdir -p data
	go run ./cmd/import-hsk -db $(or $(DB),data/vocab.db) -levels $(or $(LEVELS),1,2,3,4,5,6)

backup:
	sqlite3 data/vocab.db ".backup data/vocab_backup$(EXT).sq3"

## release: cross-compile for Raspberry Pi (arm64) and rsync to RSYNC_DEST
release:
	@test -n "$(RSYNC_DEST)" || (echo "RSYNC_DEST is not set. Copy .env.example to .env and fill it in." && exit 1)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s" -o vocab-trainer .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s" -o import-hsk ./cmd/import-hsk
	rsync -avz --progress \
	    Makefile \
		vocab-trainer \
		import-hsk \
		.env.example \
		deploy/vocab-trainer.service \
		deploy/vocab-trainer-watcher.service \
		deploy/vocab-trainer-watcher.path \
		deploy/nginx.conf \
		$(RSYNC_DEST)/

## test: run all tests (Go + JS)
test: test-go test-js

## test-go: run Go tests (uses in-memory SQLite, no server needed)
test-go:
	go test ./... -count=1

## test-js: run frontend tests with Vitest (requires Node; run 'npm install' first)
test-js:
	npm test

## clean: stop containers and remove build artifacts
clean:
	docker compose down --rmi local --volumes
	rm -f vocab-trainer import-hsk
