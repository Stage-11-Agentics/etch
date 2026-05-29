# Etch — build, test, and install the entire-agent-etch plugin.
#
# Quick start:
#   make build      # compile ./bin/entire-agent-etch
#   make test       # run the unit suite
#   make install    # install to /usr/local/bin (override with PREFIX=...)
#   make smoke      # end-to-end smoke test against the real Entire CLI

BINARY  := entire-agent-etch
PKG     := ./cmd/entire-agent-etch
BIN_DIR := bin
BIN     := $(BIN_DIR)/$(BINARY)
PREFIX  ?= /usr/local

.DEFAULT_GOAL := help

.PHONY: help build test test-density install uninstall clean smoke

help: ## Print this help
	@echo "Etch — Makefile targets:"
	@echo
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
	@echo
	@echo "Install prefix: PREFIX=$(PREFIX) (override e.g. 'PREFIX=\$$HOME/.local make install')"

build: ## Compile the binary into ./bin/entire-agent-etch
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN) $(PKG)
	@echo "built $(BIN)"

test: ## Run the unit test suite (go test ./...)
	go test ./...

test-density: ## Run the 20-concurrent-session density stress test
	go test -tags density -v ./test/density/

install: build ## Install the binary to $(PREFIX)/bin (PREFIX=/usr/local default)
	@mkdir -p "$(PREFIX)/bin"
	install -m 0755 $(BIN) "$(PREFIX)/bin/$(BINARY)"
	@echo "installed $(PREFIX)/bin/$(BINARY)"

uninstall: ## Remove the binary from $(PREFIX)/bin
	rm -f "$(PREFIX)/bin/$(BINARY)"
	@echo "removed $(PREFIX)/bin/$(BINARY)"

clean: ## Remove build artifacts (./bin)
	rm -rf $(BIN_DIR)
	@echo "cleaned $(BIN_DIR)"

smoke: ## Run the end-to-end smoke test (scripts/smoke.sh)
	bash scripts/smoke.sh
