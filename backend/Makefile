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
vet: ## Run go vet (including integration and end-to-end tests)
	go vet ./...
	go vet -tags=integration ./...
	go vet -tags=e2e ./...

# The contracts and the nearcore sandbox (version and fd-limit shim) of
# contracts/Makefile.
CONTRACTS_DIR    := $(abspath ../contracts)
SANDBOX_VERSION  := $(shell sed -n 's/^SANDBOX_VERSION := //p' $(CONTRACTS_DIR)/Makefile)
SANDBOX_SHIM_DIR := $(CONTRACTS_DIR)/target/sandbox-shim

# Runs the api and the worker against a local NEAR sandbox with the escrow
# and mock-ft contracts built from contracts/. NEAR_SANDBOX_BIN_PATH picks
# another sandbox binary; otherwise the version above is downloaded and, where
# the open-file hard limit is below the 65,535 nearcore needs, run through
# scripts/sandbox-fd-shim.sh.
.PHONY: test-e2e
test-e2e: ## Run end-to-end tests on a NEAR sandbox (needs `make compose-up`, goose and cargo-near)
	$(MAKE) -C $(CONTRACTS_DIR) build
	@bin="$${NEAR_SANDBOX_BIN_PATH:-}"; \
	if [ -z "$$bin" ]; then \
		shim=$$($(CONTRACTS_DIR)/scripts/sandbox-fd-shim.sh $(SANDBOX_VERSION) $(SANDBOX_SHIM_DIR)) || exit 1; \
		hard=$$(ulimit -H -n); \
		if [ "$$hard" != unlimited ] && [ "$$hard" -lt 65535 ]; then \
			echo "open-file hard limit $$hard < 65535: running the sandbox through scripts/sandbox-fd-shim.sh"; \
			bin=$$shim; \
		else \
			bin=$(SANDBOX_SHIM_DIR)/near-sandbox-$(SANDBOX_VERSION); \
		fi; \
	fi; \
	NEAR_SANDBOX_BIN_PATH=$$bin VECROW_WASM_DIR=$(CONTRACTS_DIR)/target/near GOOSE=$(GOOSE) \
		go test -count=1 -tags=e2e -timeout 20m -v ./e2e/...

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
