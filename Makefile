.PHONY: build run start stop restart logs dev tidy clean import import-hanzi import-cedict import-handedict import-hsk import-pinyin import-frequency fill-translations backup restore release test test-go test-js test-e2e test-all screenshots-readme screenshots-pr generate-landing

# Load .env if present (for RSYNC_DEST)
-include .env
export

## generate-landing: regenerate the landing page's feature-teaser grid from
## frontend/landing/teasers/*/teaser.json + images (service/cmd/gen-landing).
## The Dockerfile also runs this during 'make build'/'make restart', since
## those build the binary inside the image; this target covers the paths
## that build the Go binary directly on the host ('dev', 'release').
generate-landing:
	cd service && go run ./cmd/gen-landing

## build: build the Docker image (regenerates the landing page first, via the Dockerfile)
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

restart: stop build start

## logs: tail container logs
logs:
	docker compose logs -f

## dev: run locally without Docker (requires Go 1.22+)
dev: generate-landing
	mkdir -p data
	DB_PATH=data/vocab.db go run ./service

## tidy: tidy Go module dependencies
tidy:
	cd service && go mod tidy

## import: import vocabulary from a text file (FILE=voc.txt DB=data/vocab.db)
import:
	mkdir -p data
	cd service && go run ./cmd/import -db $(or $(DB),../data/vocab.db) -file $(or $(FILE),../voc.txt)

## import-hanzi: import makemeahanzi dictionary.txt for character decomposition (FILE=dictionary.txt DB=data/vocab.db)
import-hanzi:
	mkdir -p data
	cd service && go run ./cmd/import-hanzi -db $(or $(DB),../data/vocab.db) -file $(or $(FILE),../dictionary.txt)

## import-cedict: import CC-CEDICT for zh word segmentation + free EN dictionary lookups (FILE=cedict_ts.u8 DB=data/vocab.db)
import-cedict:
	mkdir -p data
	cd service && go run ./cmd/import-cedict -db $(or $(DB),../data/vocab.db) -file $(or $(FILE),../cedict_ts.u8) -lang en

## import-handedict: import HanDeDict for free DE dictionary lookups (FILE=handedict.u8 DB=data/vocab.db)
import-handedict:
	mkdir -p data
	cd service && go run ./cmd/import-cedict -db $(or $(DB),../data/vocab.db) -file $(or $(FILE),../handedict.u8) -lang de

## import-hsk: fetch and import HSK vocabulary from mandarinbean.com (LEVELS=1,2,3,4,5,6 DB=data/vocab.db)
import-hsk:
	mkdir -p data
	cd service && go run ./cmd/import-hsk -db $(or $(DB),../data/vocab.db) -levels $(or $(LEVELS),1,2,3,4,5,6)

## import-pinyin: import pinyin audio files (SOURCE=mp3-chinese-pinyin-sound/mp3 DB=data/vocab.db PINYIN_AUDIO_DIR=data/pinyin-audio)
## git clone https://github.com/davinfifield/mp3-chinese-pinyin-sound.git
## make import-pinyin SOURCE=mp3-chinese-pinyin-sound/mp3
import-pinyin:
	mkdir -p data
	cd service && go run ./cmd/import-pinyin -db $(or $(DB),../data/vocab.db) -source ../$(or $(SOURCE),mp3) -audio-dir ../$(or $(PINYIN_AUDIO_DIR),data/pinyin-audio)

## import-frequency: import a Chinese word-frequency list used to order new-word introduction (see issue #340) — the bundled list is already auto-imported by the schema migration on startup; use this to import an alternative/updated list (FILE=frequency_data.txt DB=data/vocab.db)
import-frequency:
	mkdir -p data
	cd service && go run ./cmd/import-frequency -db $(or $(DB),../data/vocab.db) -file $(or $(FILE),cmd/import-frequency/frequency_data.txt)

## funnel: print the signup → activation → retention funnel (DB=data/vocab.db MIN_ATTEMPTS=20)
funnel:
	cd service && go run ./cmd/funnel -db $(or $(DB),../data/vocab.db) -min-attempts $(or $(MIN_ATTEMPTS),20)

## fill-translations: fill missing EN/DE translations via DeepL (DEEPL_API_KEY required, DB=data/vocab.db)
fill-translations:
	mkdir -p data
	cd service && go run ./cmd/fill-translations -db $(or $(DB),../data/vocab.db) $(if $(DRY_RUN),-dry-run)

backup:
	sqlite3 data/vocab.db ".backup data/vocab_backup$(EXT).sq3"

## restore: restore the live DB from a backup file (FROM=path). Stop the server first.
restore:
	@test -n "$(FROM)" || (echo "FROM is not set. Usage: make restore FROM=data/vocab_backup.sq3" && exit 1)
	@test -f "$(FROM)" || (echo "backup file not found: $(FROM)" && exit 1)
	cp "$(FROM)" data/vocab.db
	@echo "restored data/vocab.db from $(FROM)"

## release: cross-compile for Raspberry Pi (arm64) and rsync to RSYNC_DEST
release: generate-landing
	@branch=$$(git rev-parse --abbrev-ref HEAD); \
	default=$$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's|refs/remotes/origin/||'); \
	default=$${default:-main}; \
	if [ "$$branch" != "$$default" ]; then \
		printf "WARNING: You are on branch '$$branch', not '$$default'. Release anyway? [y/N] "; \
		read answer; \
		[ "$$answer" = "y" ] || [ "$$answer" = "Y" ] || (echo "Aborted." && exit 1); \
	fi
	@test -n "$(RSYNC_DEST)" || (echo "RSYNC_DEST is not set. Copy .env.example to .env and fill it in." && exit 1)
	cd service && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s" -o ../vocab-trainer .
	cd service && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s" -o ../import-hsk ./cmd/import-hsk
	cd service && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s" -o ../import-hanzi ./cmd/import-hanzi
	cd service && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s" -o ../import-pinyin ./cmd/import-pinyin
	cd service && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s" -o ../import-frequency ./cmd/import-frequency
	cd service && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s" -o ../fill-translations ./cmd/fill-translations
	cd service && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s" -o ../import-cedict ./cmd/import-cedict
	rsync -avz --progress \
	    Makefile \
	    dictionary.txt \
		vocab-trainer \
		import-hsk \
		import-hanzi \
		import-pinyin \
		import-frequency \
		import-cedict \
		service/cmd/import-frequency/frequency_data.txt \
		fill-translations \
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
	cd service && go test ./... -count=1

## test-js: run frontend tests with Vitest (requires Node; run 'npm install' first)
test-js:
	npm test

## test-e2e: run end-to-end browser tests with Playwright (builds the Go binary first)
test-e2e:
	npx playwright test

## test-all: run all tests — Go unit tests, JS unit tests, and E2E browser tests
test-all: test-go test-js test-e2e

## screenshots-readme: regenerate README screenshots with Playwright (on-demand only, not part of test-all/CI)
##
## To screenshot against seeded data (default, fresh temp server):
##   make screenshots-readme
##
## To screenshot against the local running server as a real user:
##   make screenshots-readme USER_EMAIL=me@example.com USER_PASSWORD=secret
##   make screenshots-readme USER_EMAIL=me@example.com USER_PASSWORD=secret LOCAL_SERVER_URL=http://localhost:8080
screenshots-readme:
	@if [ -n "$(USER_EMAIL)" ]; then \
		test -n "$(USER_PASSWORD)" || (echo "USER_PASSWORD is required when USER_EMAIL is set" && exit 1); \
		USE_LOCAL_SERVER=1 LOCAL_USER_EMAIL="$(USER_EMAIL)" LOCAL_USER_PASSWORD="$(USER_PASSWORD)" \
		  LOCAL_SERVER_URL="$(or $(LOCAL_SERVER_URL),http://localhost:8080)" \
		  npx playwright test --config=playwright.screenshots.config.js; \
	else \
		npx playwright test --config=playwright.screenshots.config.js; \
	fi

## screenshots-pr: capture PR review screenshots from an e2e spec (FILE=e2e/vocab.spec.js required)
## Requires captureForPR() calls (e2e/helpers/screenshot.js) in the spec; writes PNGs to pr-screenshots/.
##
## To screenshot against seeded data (default, fresh temp server):
##   make screenshots-pr FILE=e2e/vocab.spec.js
##
## To screenshot against the local running server as a real user:
##   make screenshots-pr FILE=e2e/vocab.spec.js USER_EMAIL=me@example.com USER_PASSWORD=secret
##   make screenshots-pr FILE=e2e/vocab.spec.js USER_EMAIL=me@example.com USER_PASSWORD=secret LOCAL_SERVER_URL=http://localhost:8080
screenshots-pr:
	@test -n "$(FILE)" || (echo "FILE is not set. Usage: make screenshots-pr FILE=e2e/vocab.spec.js" && exit 1)
	@if [ -n "$(USER_EMAIL)" ]; then \
		test -n "$(USER_PASSWORD)" || (echo "USER_PASSWORD is required when USER_EMAIL is set" && exit 1); \
		USE_LOCAL_SERVER=1 LOCAL_USER_EMAIL="$(USER_EMAIL)" LOCAL_USER_PASSWORD="$(USER_PASSWORD)" \
		  LOCAL_SERVER_URL="$(or $(LOCAL_SERVER_URL),http://localhost:8080)" \
		  PR_SCREENSHOTS=1 npx playwright test --config=playwright.pr-screenshots.config.js $(FILE); \
	else \
		PR_SCREENSHOTS=1 npx playwright test --config=playwright.pr-screenshots.config.js $(FILE); \
	fi

## clean: stop containers and remove build artifacts
clean:
	docker compose down --rmi local --volumes
	rm -f vocab-trainer import-hsk
