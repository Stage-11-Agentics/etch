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

# Build identity stamped into the binary so `doctor` can flag a stale install.
# COMMIT carries a -dirty suffix when the worktree has uncommitted *tracked*
# changes. --untracked-files=no is essential: every etch-enabled repo has an
# untracked .etch/ capture dir, so counting untracked files would mark every
# build dirty and make doctor's currency warning cry-wolf in every real repo.
VERSION_PKG := github.com/Stage-11-Agentics/etch/internal/version
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null)$(shell test -n "$$(git status --porcelain --untracked-files=no 2>/dev/null)" && echo -dirty)
BUILD_DATE  := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)

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
	go build -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)
	@echo "built $(BIN) ($(COMMIT) $(BUILD_DATE))"

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
