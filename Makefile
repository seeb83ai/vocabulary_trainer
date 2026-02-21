.PHONY: build run stop logs dev tidy clean

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

## clean: stop containers and remove build artifacts
clean:
	docker compose down --rmi local --volumes
	rm -f vocab-trainer
