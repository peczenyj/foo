GO            ?= go
# Tools are installed by each developer, not pinned in go.mod: they keep
# working offline and with clean Go caches. CI installs the same versions.
# make check-tools tells you what is missing; make install-tools installs the
# Go-installed ones (golangci-lint: see its install page).
GOLANGCI_LINT ?= golangci-lint
GOLANGCI_LINT_MOD     := github.com/golangci/golangci-lint/v2
# Keep in sync with the golangci-lint-action version in .github/workflows/ci.yml.
GOLANGCI_LINT_VERSION := v2.15.0
GOLANGCI_LINT_INSTALL := https://golangci-lint.run/docs/welcome/install/
MOCKERY       ?= mockery
MOCKERY_MOD     := github.com/vektra/mockery/v3
MOCKERY_PKG     := $(MOCKERY_MOD)
MOCKERY_VERSION := v3.9.1
GOTESTSUM     ?= gotestsum
GOTESTSUM_MOD     := gotest.tools/gotestsum
GOTESTSUM_PKG     := $(GOTESTSUM_MOD)
GOTESTSUM_VERSION := v1.13.0
BENCHSTAT     ?= benchstat
BENCHSTAT_MOD     := golang.org/x/perf
BENCHSTAT_PKG     := $(BENCHSTAT_MOD)/cmd/benchstat
BENCHSTAT_VERSION := v0.0.0-20261117185222-4febdb082b2c
# vacuum lints the OpenAPI spec (api/openapi.yaml).
VACUUM        ?= vacuum
VACUUM_MOD     := github.com/daveshanley/vacuum
VACUUM_PKG     := $(VACUUM_MOD)
VACUUM_VERSION := v0.42.3
# oasdiff fails on OpenAPI changes that break clients of the last release.
OASDIFF       ?= oasdiff
OASDIFF_MOD     := github.com/oasdiff/oasdiff
OASDIFF_PKG     := $(OASDIFF_MOD)
OASDIFF_VERSION := v1.39.0
# install-tools installs these (TOOLS=gotestsum to pick).
TOOLS         ?= mockery gotestsum benchstat vacuum oasdiff
# Lint profile: full (.golangci.yml, what CI runs) or lite (.golangci.lite.yml:
# correctness only, for short, time-boxed work). A project switches with this
# one line; make lint LINT_PROFILE=lite tries it once.
LINT_PROFILE  ?= full
LINT_CONFIG    = $(if $(filter lite,$(LINT_PROFILE)),.golangci.lite.yml,.golangci.yml)
ifeq ($(filter full lite,$(LINT_PROFILE)),)
$(error LINT_PROFILE must be full or lite, not "$(LINT_PROFILE)")
endif
# The project name and module path: every other name derives from them
# (binaries, image tag, labels, coverage filter). make rename changes NAME.
NAME          ?= foo
MODULE        := $(shell awk '/^module / { print $$2; exit }' go.mod)
# The image's org.opencontainers.image.source label: the repository URL
# (github.com/owner/repo, without a /vN suffix or a subdirectory).
SOURCE        ?= https://$(shell echo '$(MODULE)' | sed -E 's|^(github\.com/[^/]+/[^/]+).*|\1|; s|/v[0-9]+$$||')
BIN           := bin/$(NAME)
DEBUG_BIN     := bin/$(NAME)-debug
# Scripts run from recipes (scripts/hurl.sh) find the binary here.
export BIN
# Recipes run scripts with "bash scripts/x.sh", never "scripts/x.sh": a copy
# made with gonew comes from a module zip, which keeps no executable bits.
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REVISION      ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
VERSION_FLAGS := -X main.version=$(VERSION) -X main.revision=$(REVISION)
LDFLAGS       := -s -w $(VERSION_FLAGS)
COVERPROFILE  ?= coverage.out
# Tests run through gotestsum; flags after -- go to go test.
# Formats: pkgname (default), testname, dots, github-actions.
GOTESTSUM_FORMAT ?= pkgname
TEST_RUNNER    = $(GOTESTSUM) --format $(GOTESTSUM_FORMAT) --
# Benchmarks: BENCH filters by name; BENCH_COUNT repeats each one so that
# benchstat can report mean ± variation instead of a single noisy run.
# Profiles need a single package: BENCH_PKG.
BENCH         ?= .
BENCH_COUNT   ?= 6
BENCH_PKG     ?= ./internal/api
# bench-compare: the git ref to compare the working tree against.
BASE          ?= devel
# Coverage is measured on our code only: generated mocks are left out.
COVER_PKGS     = $(shell $(GO) list ./... | grep -v '^$(MODULE)/mocks/')

.DEFAULT_GOAL := help
# Workflow targets (task, ci, generate) chain their steps as prerequisites,
# which make runs left to right. .NOTPARALLEL keeps that order under make -j:
# go already builds and tests in parallel on its own.
.NOTPARALLEL:
.PHONY: help task ci generate go-generate check-tools check-golangci-lint check-mockery check-gotestsum check-benchstat check-vacuum check-oasdiff install-tools build build-debug run test cover bench bench-compare bench-profile lint lint-profiles openapi-lint openapi-breaking lint-fix fmt fmt-check mod-check mocks test-functional test-integration tidy vendor docker rename rename-files clean

help: ## Show available targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  %-17s %s\n",$$1,$$2}'

task: generate tidy lint-fix fmt ci ## Before a commit: generate, tidy, fix, format, then run ci (edits files)

ci: mod-check fmt-check lint lint-profiles openapi-lint openapi-breaking cover ## All checks, read-only: mod-check, fmt-check, lint, lint-profiles, openapi-lint, openapi-breaking, cover (same as GitHub CI)

generate: go-generate mocks ## go generate ./... + regenerate mocks

go-generate: ## go generate ./...
	$(GO) generate ./...

check-tools: check-golangci-lint check-mockery check-gotestsum check-benchstat check-vacuum check-oasdiff ## Check every tool: missing → error with how to install it; other version → warning

# Each target checks only the tools it runs.
check-golangci-lint:
	@bash scripts/check-tool.sh GOLANGCI_LINT $(GOLANGCI_LINT) $(GOLANGCI_LINT_MOD) $(GOLANGCI_LINT_VERSION) "$(GOLANGCI_LINT_INSTALL)"

check-mockery:
	@bash scripts/check-tool.sh MOCKERY $(MOCKERY) $(MOCKERY_MOD) $(MOCKERY_VERSION) "make install-tools TOOLS=mockery"

check-gotestsum:
	@bash scripts/check-tool.sh GOTESTSUM $(GOTESTSUM) $(GOTESTSUM_MOD) $(GOTESTSUM_VERSION) "make install-tools TOOLS=gotestsum"

check-benchstat:
	@bash scripts/check-tool.sh BENCHSTAT $(BENCHSTAT) $(BENCHSTAT_MOD) $(BENCHSTAT_VERSION) "make install-tools TOOLS=benchstat"

check-vacuum:
	@bash scripts/check-tool.sh VACUUM $(VACUUM) $(VACUUM_MOD) $(VACUUM_VERSION) "make install-tools TOOLS=vacuum"

check-oasdiff:
	@bash scripts/check-tool.sh OASDIFF $(OASDIFF) $(OASDIFF_MOD) $(OASDIFF_VERSION) "make install-tools TOOLS=oasdiff"

install-tools: ## go install the pinned mockery, gotestsum, benchstat, vacuum and oasdiff (TOOLS=gotestsum picks; needs network)
	$(if $(filter-out mockery gotestsum benchstat vacuum oasdiff,$(TOOLS)),$(error unknown TOOLS: $(filter-out mockery gotestsum benchstat vacuum oasdiff,$(TOOLS)); known: mockery gotestsum benchstat vacuum oasdiff))
	$(if $(filter mockery,$(TOOLS)),$(GO) install $(MOCKERY_PKG)@$(MOCKERY_VERSION))
	$(if $(filter gotestsum,$(TOOLS)),$(GO) install $(GOTESTSUM_PKG)@$(GOTESTSUM_VERSION))
	$(if $(filter benchstat,$(TOOLS)),$(GO) install $(BENCHSTAT_PKG)@$(BENCHSTAT_VERSION))
	$(if $(filter vacuum,$(TOOLS)),$(GO) install $(VACUUM_PKG)@$(VACUUM_VERSION))
	$(if $(filter oasdiff,$(TOOLS)),$(GO) install $(OASDIFF_PKG)@$(OASDIFF_VERSION))

build: ## Build the server binary, bin/<NAME>
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/server

build-debug: ## Build bin/<NAME>-debug with debug symbols, no optimizations (for dlv)
	CGO_ENABLED=0 $(GO) build -gcflags='all=-N -l' -ldflags "$(VERSION_FLAGS)" -o $(DEBUG_BIN) ./cmd/server

run: ## Run the server locally (flags via ARGS, e.g. ARGS=-log-level=debug)
	$(GO) run ./cmd/server $(ARGS)

test: check-gotestsum ## Run all tests with the race detector (gotestsum; GOTESTSUM_FORMAT=testname lists each test)
	$(TEST_RUNNER) -race -count=1 ./...

cover: check-gotestsum ## Run tests with a coverage summary (mocks excluded)
	$(TEST_RUNNER) -race -covermode=atomic -coverprofile=$(COVERPROFILE) $(COVER_PKGS)
	$(GO) tool cover -func=$(COVERPROFILE) | tail -1

bench: check-benchstat ## Benchmarks (time, B/op, allocs/op) x BENCH_COUNT into bench.out, summarised by benchstat
	$(GO) test -run='^$$' -bench='$(BENCH)' -benchmem -count=$(BENCH_COUNT) ./... > bench.out || { cat bench.out; exit 1; }
	$(BENCHSTAT) bench.out

bench-compare: check-benchstat ## Benchmarks of BASE (default devel) vs the working tree, compared by benchstat
	BENCH='$(BENCH)' BENCH_COUNT=$(BENCH_COUNT) BENCHSTAT='$(BENCHSTAT)' bash scripts/bench-compare.sh $(BASE)

bench-profile: ## CPU and memory profiles of BENCH in BENCH_PKG, for go tool pprof
	$(GO) test -run='^$$' -bench='$(BENCH)' -benchmem -cpuprofile=cpu.out -memprofile=mem.out -o bench.test $(BENCH_PKG)
	@echo "profiles: go tool pprof -top bench.test cpu.out"
	@echo "          go tool pprof -top -sample_index=alloc_space bench.test mem.out"

lint: check-golangci-lint ## go vet + golangci-lint linters (LINT_PROFILE=full|lite)
	$(GO) vet ./...
	$(GOLANGCI_LINT) run --config $(LINT_CONFIG) ./...

lint-profiles: check-golangci-lint ## Check the lite lint profile is a subset of full, and that the code passes it
	GOLANGCI_LINT='$(GOLANGCI_LINT)' bash scripts/lint-profiles.sh
	$(GOLANGCI_LINT) run --config .golangci.lite.yml ./...

openapi-lint: check-vacuum ## Lint the OpenAPI spec, api/openapi.yaml (vacuum; warnings fail)
	$(VACUUM) lint --details --ruleset api/vacuum.yaml --fail-severity warn api/openapi.yaml

openapi-breaking: check-oasdiff ## Fail on OpenAPI changes that break clients of the last vX.Y.Z release (oasdiff)
	OASDIFF='$(OASDIFF)' bash scripts/openapi-breaking.sh

lint-fix: check-golangci-lint ## go fix + golangci-lint --fix (edits files)
	$(GO) fix ./...
	$(GOLANGCI_LINT) run --config $(LINT_CONFIG) --fix ./...

fmt: check-golangci-lint ## Format the code (gofumpt + gci)
	$(GOLANGCI_LINT) fmt --config $(LINT_CONFIG) ./...

fmt-check: check-golangci-lint ## Fail if any file is not formatted
	$(GOLANGCI_LINT) fmt --config $(LINT_CONFIG) --diff ./...

mod-check: ## Fail if go.mod/go.sum need tidying (read-only), then verify
	$(GO) mod tidy -diff
	$(GO) mod verify

test-functional: build ## Functional tests: the hurl suite against a fresh bin/<NAME> (needs hurl)
	bash scripts/hurl.sh

test-integration: check-gotestsum ## Integration tests (TestIntegration_*, build tag integration; may need Docker)
	$(TEST_RUNNER) -race -count=1 -tags integration -run '^TestIntegration' ./...

mocks: check-mockery ## Regenerate mocks/ with mockery (.mockery.yaml)
	$(MOCKERY)

tidy: ## go mod tidy + verify
	$(GO) mod tidy && $(GO) mod verify

vendor: ## go mod tidy + vendor + verify (vendor/ is git-ignored)
	$(GO) mod tidy && $(GO) mod vendor && $(GO) mod verify

docker: ## Build the Docker image
	docker build --build-arg NAME=$(NAME) --build-arg SOURCE=$(SOURCE) \
		--build-arg VERSION=$(VERSION) --build-arg REVISION=$(REVISION) \
		--build-arg CREATED=$$(date -u +%Y-%m-%dT%H:%M:%SZ) -t $(NAME):$(VERSION) .

rename: rename-files generate tidy ## Adapt the template: make rename MODULE=<module path> NAME=<name>

rename-files:
	$(if $(filter command line,$(origin MODULE)),,$(error make rename needs MODULE=<new module path>, e.g. example.com/acme/widget))
	$(if $(filter command line,$(origin NAME)),,$(error make rename needs NAME=<new name>, e.g. widget))
	bash scripts/rename.sh '$(MODULE)' '$(NAME)'

print-%: # make print-BIN: print a variable (scripts ask make instead of repeating its values)
	@echo '$($*)'

clean: ## Remove build artefacts
	rm -rf bin coverage.out bench.out bench-base.out bench.test cpu.out mem.out
