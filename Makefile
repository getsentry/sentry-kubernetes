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
	go test -v -count=1 -race -timeout 60s ./...
.PHONY: test

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
