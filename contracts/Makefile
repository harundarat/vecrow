CARGO ?= cargo
WASM_DIR := target/near
CONTRACTS := vecrow-escrow mock-ft

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
	$(CARGO) near build non-reproducible-wasm --locked --manifest-path escrow/Cargo.toml --out-dir $(WASM_DIR)/escrow
	$(CARGO) near build non-reproducible-wasm --locked --manifest-path mock-ft/Cargo.toml --out-dir $(WASM_DIR)/mock-ft

.PHONY: test-unit
test-unit: ## Unit tests (no sandbox)
	$(CARGO) test --workspace --exclude vecrow-integration-tests

# nearcore sandbox run by near-workspaces 0.24 (near-sandbox 0.4).
SANDBOX_VERSION := 2.14.0
SANDBOX_SHIM_DIR := $(CURDIR)/target/sandbox-shim

.PHONY: test-sandbox
test-sandbox: build ## near-workspaces sandbox tests (downloads the sandbox binary on first run)
	@hard=$$(ulimit -H -n); \
	if [ -z "$${NEAR_SANDBOX_BIN_PATH:-}" ] && [ "$$(uname -s)" = Linux ] \
		&& [ "$$hard" != unlimited ] && [ "$$hard" -lt 65535 ]; then \
		echo "open-file hard limit $$hard < 65535: running the sandbox through scripts/sandbox-fd-shim.sh"; \
		NEAR_SANDBOX_BIN_PATH=$$(scripts/sandbox-fd-shim.sh $(SANDBOX_VERSION) $(SANDBOX_SHIM_DIR)) || exit 1; \
		export NEAR_SANDBOX_BIN_PATH; \
	fi; \
	$(CARGO) test -p vecrow-integration-tests

.PHONY: test
test: test-unit test-sandbox ## Unit and sandbox tests

.PHONY: clean
clean: ## Remove build output
	$(CARGO) clean
