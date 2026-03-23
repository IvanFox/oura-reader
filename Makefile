.PHONY: build run test lint clean docker docker-run help

build:
	go build -o bin/oura-reader ./cmd/oura-reader

run: build
	./bin/oura-reader serve

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -rf bin/

docker:
	docker build -t oura-reader .

docker-run:
	docker compose up -d

docker-stop:
	docker compose down

help:
	@echo "build      - Build the binary"
	@echo "run        - Build and run the server"
	@echo "test       - Run all tests"
	@echo "lint       - Run go vet"
	@echo "clean      - Remove build artifacts"
	@echo "docker     - Build Docker image"
	@echo "docker-run - Start via docker compose"
	@echo "docker-stop- Stop via docker compose"
