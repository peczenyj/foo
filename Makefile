GO            ?= go
GOLANGCI_LINT ?= golangci-lint
BIN           := bin/foo
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REVISION      ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
LDFLAGS       := -s -w -X main.version=$(VERSION) -X main.revision=$(REVISION)

.DEFAULT_GOAL := help
.PHONY: help build run test cover lint fmt fmt-check tidy clean

help: ## Show available targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  %-10s %s\n",$$1,$$2}'

build: ## Build the server binary into bin/
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/server

run: ## Run the server locally (flags via ARGS, e.g. ARGS=-log-level=debug)
	$(GO) run ./cmd/server $(ARGS)

test: ## Run all tests with the race detector
	$(GO) test -race -count=1 ./...

cover: ## Run tests with a coverage summary
	$(GO) test -race -covermode=atomic -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -1

lint: ## go vet + golangci-lint linters
	$(GO) vet ./...
	$(GOLANGCI_LINT) run ./...

fmt: ## Format the code (gofumpt + gci)
	$(GOLANGCI_LINT) fmt ./...

fmt-check: ## Fail if any file is not formatted
	$(GOLANGCI_LINT) fmt --diff ./...

tidy: ## go mod tidy + verify
	$(GO) mod tidy && $(GO) mod verify

clean: ## Remove build artefacts
	rm -rf bin coverage.out
