.PHONY: build run test clean dev lint

BINARY=oma
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

build:
	CGO_ENABLED=1 go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/oma

run: build
	./bin/$(BINARY) server start

dev:
	go run ./cmd/oma server start

test:
	go test ./... -v

lint:
	golangci-lint run

clean:
	rm -rf bin/ data/

migrate:
	go run ./cmd/oma migrate up

sqlc:
	sqlc generate

docker:
	docker compose up --build -d

docker-down:
	docker compose down
