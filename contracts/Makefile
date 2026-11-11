# Vecrow contracts — run `make help` for the list of targets.

CARGO ?= cargo
WASM_DIR := target/near
CONTRACTS := mock-ft

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

.PHONY: fmt
fmt: ## Format all crates
	$(CARGO) fmt --all

.PHONY: fmt-check
fmt-check: ## Check formatting without changing files
	$(CARGO) fmt --all -- --check

.PHONY: lint
lint: ## Clippy with warnings as errors, for the host and the wasm target
	$(CARGO) clippy --workspace --all-targets -- -D warnings
	$(CARGO) clippy $(addprefix -p ,$(CONTRACTS)) --target wasm32-unknown-unknown -- -D warnings

.PHONY: build
build: ## Build the contract wasm files into target/near/ with cargo-near
	$(CARGO) near build non-reproducible-wasm --locked --manifest-path mock-ft/Cargo.toml --out-dir $(WASM_DIR)/mock-ft

.PHONY: test-unit
test-unit: ## Unit tests (no sandbox)
	$(CARGO) test --workspace --exclude vecrow-integration-tests

.PHONY: test-sandbox
test-sandbox: build ## near-workspaces sandbox tests (downloads the sandbox binary on first run)
	$(CARGO) test -p vecrow-integration-tests

.PHONY: test
test: test-unit test-sandbox ## Unit and sandbox tests

.PHONY: clean
clean: ## Remove build output
	$(CARGO) clean
