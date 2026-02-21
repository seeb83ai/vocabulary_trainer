.PHONY: build run stop logs dev tidy clean import

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

## clean: stop containers and remove build artifacts
clean:
	docker compose down --rmi local --volumes
	rm -f vocab-trainer
