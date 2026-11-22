GO            ?= go
# golangci-lint is the only tool to install (make check-tools tells you what
# is missing); CI pins the same version.
GOLANGCI_LINT ?= golangci-lint
GOLANGCI_LINT_MOD     := github.com/golangci/golangci-lint/v2
# Keep in sync with .github/workflows/ci.yml and .gitlab-ci.yml.
GOLANGCI_LINT_VERSION := v2.15.0
GOLANGCI_LINT_INSTALL := https://golangci-lint.run/docs/welcome/install/
# The project name and module path: every other name derives from them
# (binaries, image tag, labels).
NAME          ?= foo
MODULE        := $(shell awk '/^module / { print $$2; exit }' go.mod)
# The image's org.opencontainers.image.source label: the repository URL
# (github.com/owner/repo, without a /vN suffix or a subdirectory).
SOURCE        ?= https://$(shell echo '$(MODULE)' | sed -E 's|^(github\.com/[^/]+/[^/]+).*|\1|; s|/v[0-9]+$$||')
BIN           := bin/$(NAME)
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REVISION      ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
LDFLAGS       := -s -w -X main.version=$(VERSION) -X main.revision=$(REVISION)
COVERPROFILE  ?= coverage.out

.DEFAULT_GOAL := help
# Workflow targets (task, ci) chain their steps as prerequisites, which make
# runs left to right. .NOTPARALLEL keeps that order under make -j: go already
# builds and tests in parallel on its own.
.NOTPARALLEL:
.PHONY: help task ci check-tools check-golangci-lint build run test cover lint lint-fix fmt fmt-check mod-check tidy docker clean

help: ## Show available targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  %-12s %s\n",$$1,$$2}'

task: tidy lint-fix fmt ci ## Before a commit: tidy, fix, format, then run ci (edits files)

ci: mod-check fmt-check lint cover ## All checks, read-only: mod-check, fmt-check, lint, cover (same as CI)

check-tools: check-golangci-lint ## Check golangci-lint: missing → error with how to install it; other version → warning

check-golangci-lint:
	@bash scripts/check-tool.sh GOLANGCI_LINT $(GOLANGCI_LINT) $(GOLANGCI_LINT_MOD) $(GOLANGCI_LINT_VERSION) "$(GOLANGCI_LINT_INSTALL)"

build: ## Build the server binary, bin/<NAME>
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/server

run: ## Run the server locally (flags via ARGS, e.g. ARGS=-log-level=debug)
	$(GO) run ./cmd/server $(ARGS)

test: ## Run all tests with the race detector
	$(GO) test -race -count=1 ./...

cover: ## Run tests with a coverage summary
	$(GO) test -race -covermode=atomic -coverprofile=$(COVERPROFILE) ./...
	$(GO) tool cover -func=$(COVERPROFILE) | tail -1

lint: check-golangci-lint ## go vet + golangci-lint
	$(GO) vet ./...
	$(GOLANGCI_LINT) run ./...

lint-fix: check-golangci-lint ## go fix + golangci-lint --fix (edits files)
	$(GO) fix ./...
	$(GOLANGCI_LINT) run --fix ./...

fmt: check-golangci-lint ## Format the code (gofumpt + gci)
	$(GOLANGCI_LINT) fmt ./...

fmt-check: check-golangci-lint ## Fail if any file is not formatted
	$(GOLANGCI_LINT) fmt --diff ./...

mod-check: ## Fail if go.mod/go.sum need tidying (read-only), then verify
	$(GO) mod tidy -diff
	$(GO) mod verify

tidy: ## go mod tidy + verify
	$(GO) mod tidy && $(GO) mod verify

docker: ## Build the Docker image
	docker build --build-arg NAME=$(NAME) --build-arg SOURCE=$(SOURCE) \
		--build-arg VERSION=$(VERSION) --build-arg REVISION=$(REVISION) \
		--build-arg CREATED=$$(date -u +%Y-%m-%dT%H:%M:%SZ) -t $(NAME):$(VERSION) .

print-%: # make print-BIN: print a variable
	@echo '$($*)'

clean: ## Remove build artefacts
	rm -rf bin coverage.out
