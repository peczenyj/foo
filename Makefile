GO            ?= go
GOLANGCI_LINT ?= golangci-lint
# golangci-lint is installed by each developer; CI pins the same version
# (.github/workflows/ci.yml). make check-tools tells you what to install.
GOLANGCI_LINT_VERSION := 2.15.0
GOLANGCI_LINT_INSTALL := https://golangci-lint.run/docs/welcome/install/
BIN           := bin/foo
DEBUG_BIN     := bin/foo-debug
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REVISION      ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
VERSION_FLAGS := -X main.version=$(VERSION) -X main.revision=$(REVISION)
LDFLAGS       := -s -w $(VERSION_FLAGS)
COVERPROFILE  ?= coverage.out
# Tests run through gotestsum (a go tool, pinned in go.mod); flags after --
# go to go test. Formats: pkgname (default), testname, dots, github-actions.
GOTESTSUM_FORMAT ?= pkgname
GOTESTSUM      = $(GO) tool gotestsum --format $(GOTESTSUM_FORMAT) --
# Benchmarks: BENCH filters by name; BENCH_COUNT repeats each one so that
# benchstat can report mean ± variation instead of a single noisy run.
# Profiles need a single package: BENCH_PKG.
BENCH         ?= .
BENCH_COUNT   ?= 6
BENCH_PKG     ?= ./internal/api
# bench-compare: the git ref to compare the working tree against.
BASE          ?= devel
# Coverage is measured on our code only: generated mocks are left out.
COVER_PKGS     = $(shell $(GO) list ./... | grep -v '/mocks$$')
# Steps run in sequence, never as prerequisites: under make -j, prerequisites
# may run in parallel, and generate must finish before anything else.
STEP          := $(MAKE) --no-print-directory

.DEFAULT_GOAL := help
.PHONY: help task ci generate check-tools build build-debug run test cover bench bench-compare bench-smoke bench-profile lint lint-fix fmt fmt-check mod-check mocks hurl tidy vendor docker clean

help: ## Show available targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  %-12s %s\n",$$1,$$2}'

task: ## Before a commit: generate, tidy, fix, format, then run ci (edits files)
	$(STEP) generate
	$(STEP) tidy
	$(STEP) lint-fix
	$(STEP) fmt
	$(STEP) ci

ci: ## All checks, read-only: mod-check, fmt-check, lint, cover, bench-smoke (same as GitHub CI)
	$(STEP) mod-check
	$(STEP) fmt-check
	$(STEP) lint
	$(STEP) cover
	$(STEP) bench-smoke

generate: ## go generate ./... + regenerate mocks
	$(GO) generate ./...
	$(STEP) mocks

check-tools: ## Fail with install instructions if golangci-lint is missing
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || { \
		echo "error: golangci-lint not found ($(GOLANGCI_LINT))." >&2; \
		echo "Install golangci-lint v$(GOLANGCI_LINT_VERSION), the version CI uses:" >&2; \
		echo "  $(GOLANGCI_LINT_INSTALL)" >&2; \
		echo "or point to another binary: make <target> GOLANGCI_LINT=/path/to/golangci-lint" >&2; \
		exit 1; }
	@v=$$($(GOLANGCI_LINT) version --short 2>/dev/null); \
	[ "$$v" = "$(GOLANGCI_LINT_VERSION)" ] || \
		echo "warning: golangci-lint $$v is installed, CI uses $(GOLANGCI_LINT_VERSION): results may differ ($(GOLANGCI_LINT_INSTALL))" >&2

build: ## Build the server binary into bin/
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/server

build-debug: ## Build bin/foo-debug with debug symbols, no optimizations (for dlv)
	CGO_ENABLED=0 $(GO) build -gcflags='all=-N -l' -ldflags "$(VERSION_FLAGS)" -o $(DEBUG_BIN) ./cmd/server

run: ## Run the server locally (flags via ARGS, e.g. ARGS=-log-level=debug)
	$(GO) run ./cmd/server $(ARGS)

test: ## Run all tests with the race detector (gotestsum; GOTESTSUM_FORMAT=testname lists each test)
	$(GOTESTSUM) -race -count=1 ./...

cover: ## Run tests with a coverage summary (mocks excluded)
	$(GOTESTSUM) -race -covermode=atomic -coverprofile=$(COVERPROFILE) $(COVER_PKGS)
	$(GO) tool cover -func=$(COVERPROFILE) | tail -1

bench: ## Benchmarks (time, B/op, allocs/op) x BENCH_COUNT, summarised by benchstat into bench.out
	$(GO) test -run='^$$' -bench='$(BENCH)' -benchmem -count=$(BENCH_COUNT) ./... > bench.out || { cat bench.out; exit 1; }
	$(GO) tool benchstat bench.out

bench-compare: ## Benchmarks of BASE (default devel) vs the working tree, compared by benchstat
	BENCH='$(BENCH)' BENCH_COUNT=$(BENCH_COUNT) scripts/bench-compare.sh $(BASE)

bench-smoke: ## Run every benchmark once: they compile and run (CI; timings not judged)
	$(GO) test -run='^$$' -bench=. -benchtime=1x ./...

bench-profile: ## CPU and memory profiles of BENCH in BENCH_PKG, for go tool pprof
	$(GO) test -run='^$$' -bench='$(BENCH)' -benchmem -cpuprofile=cpu.out -memprofile=mem.out -o bench.test $(BENCH_PKG)
	@echo "profiles: go tool pprof -top bench.test cpu.out"
	@echo "          go tool pprof -top -sample_index=alloc_space bench.test mem.out"

lint: check-tools ## go vet + golangci-lint linters
	$(GO) vet ./...
	$(GOLANGCI_LINT) run ./...

lint-fix: check-tools ## go fix + golangci-lint --fix (edits files)
	$(GO) fix ./...
	$(GOLANGCI_LINT) run --fix ./...

fmt: check-tools ## Format the code (gofumpt + gci)
	$(GOLANGCI_LINT) fmt ./...

fmt-check: check-tools ## Fail if any file is not formatted
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
	rm -rf bin coverage.out bench.out bench-base.out bench.test cpu.out mem.out
