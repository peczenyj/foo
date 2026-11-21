GO            ?= go
GOLANGCI_LINT ?= golangci-lint
BIN           := bin/foo
DEBUG_BIN     := bin/foo-debug
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REVISION      ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
VERSION_FLAGS := -X main.version=$(VERSION) -X main.revision=$(REVISION)
LDFLAGS       := -s -w $(VERSION_FLAGS)
COVERPROFILE  ?= coverage.out
# Coverage is measured on our code only: generated mocks are left out.
COVER_PKGS     = $(shell $(GO) list ./... | grep -v '/mocks$$')
# Steps run in sequence, never as prerequisites: under make -j, prerequisites
# may run in parallel, and generate must finish before anything else.
STEP          := $(MAKE) --no-print-directory

.DEFAULT_GOAL := help
.PHONY: help task ci generate build build-debug run test cover lint lint-fix fmt fmt-check mod-check mocks hurl tidy vendor docker clean

help: ## Show available targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  %-12s %s\n",$$1,$$2}'

task: ## Before a commit: generate, tidy, fix, format, then run ci (edits files)
	$(STEP) generate
	$(STEP) tidy
	$(STEP) lint-fix
	$(STEP) fmt
	$(STEP) ci

ci: ## All checks, read-only: mod-check, fmt-check, lint, cover (same as GitHub CI)
	$(STEP) mod-check
	$(STEP) fmt-check
	$(STEP) lint
	$(STEP) cover

generate: ## go generate ./... + regenerate mocks
	$(GO) generate ./...
	$(STEP) mocks

build: ## Build the server binary into bin/
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/server

build-debug: ## Build bin/foo-debug with debug symbols, no optimizations (for dlv)
	CGO_ENABLED=0 $(GO) build -gcflags='all=-N -l' -ldflags "$(VERSION_FLAGS)" -o $(DEBUG_BIN) ./cmd/server

run: ## Run the server locally (flags via ARGS, e.g. ARGS=-log-level=debug)
	$(GO) run ./cmd/server $(ARGS)

test: ## Run all tests with the race detector
	$(GO) test -race -count=1 ./...

cover: ## Run tests with a coverage summary (mocks excluded)
	$(GO) test -race -covermode=atomic -coverprofile=$(COVERPROFILE) $(COVER_PKGS)
	$(GO) tool cover -func=$(COVERPROFILE) | tail -1

lint: ## go vet + golangci-lint linters
	$(GO) vet ./...
	$(GOLANGCI_LINT) run ./...

lint-fix: ## go fix + golangci-lint --fix (edits files)
	$(GO) fix ./...
	$(GOLANGCI_LINT) run --fix ./...

fmt: ## Format the code (gofumpt + gci)
	$(GOLANGCI_LINT) fmt ./...

fmt-check: ## Fail if any file is not formatted
	$(GOLANGCI_LINT) fmt --diff ./...

mod-check: ## Fail if go.mod/go.sum need tidying (read-only), then verify
	$(GO) mod tidy -diff
	$(GO) mod verify

hurl: ## Run the hurl suite against a fresh server (needs hurl)
	scripts/hurl.sh

mocks: ## Regenerate mocks with mockery (.mockery.yml)
	$(GO) tool mockery

tidy: ## go mod tidy + verify
	$(GO) mod tidy && $(GO) mod verify

vendor: ## go mod tidy + vendor + verify (vendor/ is git-ignored)
	$(GO) mod tidy && $(GO) mod vendor && $(GO) mod verify

docker: ## Build the Docker image
	docker build --build-arg VERSION=$(VERSION) --build-arg REVISION=$(REVISION) --build-arg CREATED=$$(date -u +%Y-%m-%dT%H:%M:%SZ) -t foo:$(VERSION) .

clean: ## Remove build artefacts
	rm -rf bin coverage.out
