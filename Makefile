.DEFAULT_GOAL := help

MKFILE_PATH := $(abspath $(lastword $(MAKEFILE_LIST)))
MKFILE_DIR := $(dir $(MKFILE_PATH))
# In seconds
TIMEOUT = 60

# Parse Makefile and display the help
help: ## Show help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
.PHONY: help

docker-build:
	docker build -t sentry-kubernetes .
.PHONY: docker-build

upload-image-kind: docker-build
	kind load docker-image sentry-kubernetes
.PHONY: upload-image-kind

test: ## Run tests
	go test -v -count=1 -race -timeout $(TIMEOUT)s ./...
.PHONY: test

# Coverage
COVERAGE_MODE    	= atomic
COVERAGE_PROFILE 	= coverage.out
COVERAGE_REPORT_DIR = .coverage
COVERAGE_REPORT_DIR_ABS  = $(MKFILE_DIR)/$(COVERAGE_REPORT_DIR)
COVERAGE_REPORT_FILE_ABS = $(COVERAGE_REPORT_DIR_ABS)/$(COVERAGE_PROFILE)
$(COVERAGE_REPORT_DIR):
	mkdir -p $(COVERAGE_REPORT_DIR)
clean-report-dir: $(COVERAGE_REPORT_DIR)
	test $(COVERAGE_REPORT_DIR) && rm -f $(COVERAGE_REPORT_DIR)/*
test-coverage: $(COVERAGE_REPORT_DIR) clean-report-dir  ## Test with coverage enabled
	set -e ; \
	go test -count=1 -timeout $(TIMEOUT)s -coverpkg=./... -covermode=$(COVERAGE_MODE) -coverprofile="$(COVERAGE_REPORT_FILE_ABS)" ./... ; \
	go tool cover -html="$(COVERAGE_REPORT_FILE_ABS)" -o "$(COVERAGE_REPORT_DIR_ABS)/coverage.html";
.PHONY: test-coverage clean-report-dir

build: ## Build the module
	go build ./...
.PHONY: build

mod-tidy: ## Check go.mod tidiness
	go mod tidy; \
		git diff --exit-code;
.PHONY: mod-tidy

vet: ## Run "go vet"
	go vet ./...
.PHONY: vet

fmt: ## Run "go fmt"
	go fmt ./...; \
		git diff --exit-code;
.PHONY: fmt
