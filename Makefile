.PHONY: build test lint migrate run-indexer run-api clean

# Binary output directory
BIN_DIR := bin

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOMOD := $(GOCMD) mod

# Database
DB_URL ?= postgres://crosschain:crosschain@localhost:5432/crosschain_archive?sslmode=disable
MIGRATIONS_DIR := migrations

build:
	$(GOBUILD) -o $(BIN_DIR)/indexer ./cmd/indexer
	$(GOBUILD) -o $(BIN_DIR)/api ./cmd/api

test:
	$(GOTEST) -v -race ./...

lint:
	golangci-lint run ./...

migrate:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" up

migrate-down:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" down

run-indexer:
	$(GOCMD) run ./cmd/indexer

run-api:
	$(GOCMD) run ./cmd/api

clean:
	rm -rf $(BIN_DIR)

tidy:
	$(GOMOD) tidy
