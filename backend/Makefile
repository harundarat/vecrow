# Load .env (if present) and export its variables to every recipe.
-include .env
export

MODULE         := github.com/harundarat/vecrow/backend
BIN_DIR        := bin
MIGRATIONS_DIR := migrations
GOOSE          ?= goose
SQLC           ?= sqlc
VERSION        ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT         ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS        := -X $(MODULE)/internal/buildinfo.Version=$(VERSION) -X $(MODULE)/internal/buildinfo.Commit=$(COMMIT)

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

## --- Run & build ------------------------------------------------------------

.PHONY: run-api
run-api: ## Run the API server
	go run -ldflags "$(LDFLAGS)" ./cmd/api

.PHONY: run-worker
run-worker: ## Run the worker (scheduler + queue consumers)
	go run -ldflags "$(LDFLAGS)" ./cmd/worker

.PHONY: build
build: ## Build both binaries into bin/
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o $(BIN_DIR)/ ./cmd/...

.PHONY: docker-build
docker-build: ## Build the container image
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t vecrow-backend:$(VERSION) .

## --- Quality ----------------------------------------------------------------

.PHONY: test
test: ## Run unit tests
	go test -race -count=1 ./...

.PHONY: test-integration
test-integration: ## Run unit + integration tests (needs `make compose-up`)
	go test -race -count=1 -tags=integration ./...

.PHONY: vet
vet: ## Run go vet (including integration tests)
	go vet ./...
	go vet -tags=integration ./...

.PHONY: fmt
fmt: ## Format the code
	gofmt -s -w .

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	go mod tidy

## --- Database ---------------------------------------------------------------

.PHONY: sqlc
sqlc: ## Generate Go code from internal/repository/queries
	$(SQLC) generate

.PHONY: migrate-up
migrate-up: ## Apply all pending migrations
	$(GOOSE) -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## Roll back the latest migration
	$(GOOSE) -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" down

.PHONY: migrate-status
migrate-status: ## Show migration status
	$(GOOSE) -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" status

.PHONY: migrate-create
migrate-create: ## Create a migration: make migrate-create name=create_accounts
	@test -n "$(name)" || (echo "usage: make migrate-create name=<name>" && exit 1)
	$(GOOSE) -dir $(MIGRATIONS_DIR) -s create $(name) sql

## --- Local infrastructure ---------------------------------------------------

.PHONY: compose-up
compose-up: ## Start postgres, redis and rabbitmq and wait until healthy
	docker compose up -d --wait

.PHONY: compose-down
compose-down: ## Stop the local infrastructure (keeps volumes)
	docker compose down
