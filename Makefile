# =============================================================================
# SENTRY KUBERNETES AGENT MAKEFILE
# =============================================================================
# This Makefile provides automation for building, testing, and developing
# the Sentry Kubernetes Agent. Run 'make help' to see all available commands.
# =============================================================================

# Default target - show help when running 'make' without arguments
.DEFAULT_GOAL := help

# In seconds
TIMEOUT = 60

# =============================================================================
# SETUP & INSTALLATION
# =============================================================================

## Initialize project for development (installs all dependencies)
.PHONY: init
init:
	@if [ "$$(uname)" = "Darwin" ]; then \
		echo "Darwin detected."; \
		$(MAKE) init-darwin; \
	elif [ "$$(uname)" = "Linux" ]; then \
		echo "Linux detected."; \
		$(MAKE) init-linux; \
	else \
		echo "Not running on Darwin or Linux."; \
		exit 1; \
	fi
	$(MAKE) install
	pre-commit install

.PHONY: init-darwin
init-darwin:
	@if ! command -v brew >/dev/null 2>&1; then \
		echo "Homebrew not detected. Skipping system dependency installation."; \
	else \
		echo "Homebrew detected. Installing system dependencies..."; \
		brew bundle; \
	fi

.PHONY: init-linux
init-linux:
	@if ! command -v dprint >/dev/null 2>&1; then \
		echo "dprint not detected. Please install: curl -fsSL https://dprint.dev/install.sh | sh"; \
	fi

## Install and tidy Go module dependencies
.PHONY: install
install:
	go mod tidy

# =============================================================================
# BUILDING
# =============================================================================

## Build the Go module
.PHONY: build
build:
	go build ./...

## Build production binary optimized for deployment
.PHONY: build-dist
build-dist:
	mkdir -p dist
	go build -ldflags="-s -w" -trimpath -o dist/sentry-kubernetes ./cmd/agent

## Build Docker image
.PHONY: build-docker
build-docker:
	docker build -t sentry-kubernetes .

## Build Docker image and load into kind cluster
.PHONY: upload-image-kind
upload-image-kind: build-docker
	kind load docker-image sentry-kubernetes

# =============================================================================
# TESTING & QUALITY ASSURANCE
# =============================================================================

## Run all tests
.PHONY: test
test:
	go test -v -count=1 -race -timeout $(TIMEOUT)s ./...

## Run tests with coverage report
.PHONY: test-coverage
test-coverage:
	mkdir -p .coverage
	go test -count=1 -race -timeout $(TIMEOUT)s \
		-coverpkg=./... \
		-covermode=atomic \
		-coverprofile=.coverage/coverage.out \
		./...
	go tool cover -html=.coverage/coverage.out -o .coverage/coverage.html

## Run static analysis (go vet)
.PHONY: vet
vet:
	go vet ./...

## Lint using golangci-lint
.PHONY: lint
lint:
	golangci-lint run -v $(ARGS)

## Lint and apply fixes (when applicable)
.PHONY: lint-fix
lint-fix: ARGS=--fix
lint-fix: lint

# =============================================================================
# FORMATTING & MAINTENANCE
# =============================================================================

## Format code and tidy modules
.PHONY: format
format:
	go mod tidy
	go fmt ./...
	dprint fmt

## Check go.mod tidiness (fails if not tidy)
.PHONY: mod-tidy
mod-tidy:
	go mod tidy
	git diff --exit-code

## Check formatting (fails if not formatted)
.PHONY: fmt-check
fmt-check:
	go fmt ./...
	git diff --exit-code

## Update all dependencies to latest compatible versions
.PHONY: upgrade-deps
upgrade-deps:
	go get -u ./...
	$(MAKE) format

# =============================================================================
# HELP & DOCUMENTATION
# =============================================================================

## Show this help message with all available commands
.PHONY: help
help:
	@echo "============================================================================="
	@echo "SENTRY KUBERNETES AGENT - DEVELOPMENT COMMANDS"
	@echo "============================================================================="
	@echo ""
	@awk 'BEGIN { desc = ""; target = "" } \
	/^## / { desc = substr($$0, 4) } \
	/^\.PHONY: / && desc != "" { \
		target = $$2; \
		printf "\033[36m%-20s\033[0m %s\n", target, desc; \
		desc = ""; target = "" \
	}' $(MAKEFILE_LIST)
	@echo ""
	@echo "Use 'make <command>' to run any command above."
	@echo ""
