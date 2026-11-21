# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

- `make task`: run before committing. generate → tidy → lint-fix → fmt → ci; it edits files. `make ci` is read-only (mod-check, fmt-check, lint, cover) and mirrors GitHub CI. Keep `ci` read-only: anything that fixes belongs in `task`.
- `make test`: all tests with `-race`, through gotestsum (`go tool gotestsum`, pinned in `go.mod`; `GOTESTSUM_FORMAT=testname|dots|github-actions`, default `pkgname`). Single test: `go tool gotestsum -- -race -run '^TestAPI$/should_reject_POST' ./internal/api/` (plain `go test` works too).
- golangci-lint is not vendored: developers install it (v2.15.0, `GOLANGCI_LINT_VERSION` in the Makefile, kept in sync with `ci.yml`). `make check-tools` (a prerequisite of lint/lint-fix/fmt/fmt-check) fails with the install URL if it is missing and warns on a version mismatch. mockery and gotestsum stay `go tool`s.
- `make lint`: `go vet` + golangci-lint linters. `make fmt` rewrites files (gofumpt + gci); `make fmt-check` only reports and is what CI runs.
- Test layers: unit (`handler_test.go`, mocked `Fooer`), component (`component_test.go`, real `Fooer`, in-process HTTP; not "integration"), integration (`//go:build integration`, `TestIntegration_*`, external dependencies such as Docker containers; `make test-integration`, never run by `make test`; `.golangci.yml` lints the tag), functional (hurl, `make test-functional`).
- `make test-functional`: builds `bin/foo`, starts it on a free port (`-port=0`, the bound address is read from the `"msg":"listening"` JSON log line), runs `test/hurl/*.hurl`, stops it. `scripts/hurl.sh <base-url>` targets a running server.
- `make cover`: coverage without `internal/api/mocks` (filtered in `COVER_PKGS`; CI runs `make cover COVERPROFILE=coverage.txt`, and `codecov.yml` ignores the mocks too).
- `make bench` (`BENCH=` filter, `BENCH_COUNT=6`): `-benchmem` runs summarised by `go tool benchstat` into `bench.out`; `make bench-profile` writes cpu/mem profiles for `BENCH_PKG`; `make bench-smoke` (in `make ci`) runs each benchmark once. `make bench-compare BASE=<ref>` benchmarks the ref in a temporary git worktree and the working tree, then `benchstat` compares them (`scripts/bench-compare.sh`). Benchmarks are for people to read: there is deliberately no automated performance or allocation assertion.
- `make mocks`: regenerate `internal/api/mocks` with `go tool mockery` (pinned in `go.mod`, configured in `.mockery.yml`). Run it after changing `foo.Fooer`; CI fails on stale mocks. Never hand-edit the mocks.
- `make vendor`: tidy + vendor + verify. `vendor/` is git-ignored and must never be committed; while it exists locally, Go builds from it.
- `make build-debug`: `bin/foo-debug` keeps DWARF and the symbol table, with `-N -l` (no optimizations or inlining) and no `-trimpath`, for `dlv exec bin/foo-debug`. `make build` is the stripped release binary.
- `make run ARGS="-log-level=debug -log-format=json"`, `make docker`.

## Architecture

`cmd/server/main.go` `run()` → `internal/config.Parse` (flags, with `FOO_*` env vars as defaults; flag > env > default; every invalid value reported, exit 2) → `cfg.NewLogger` (slog text/json) → `api.New(foo.Bar{})` → `RunServer` (listens first, logs the bound address, graceful shutdown on SIGINT/SIGTERM or ctx cancel).

- `internal/foo`: the `Fooer` interface (`Foo(ctx) (string, error)`) and the real implementation `Bar`.
- `internal/api`: the `http.Handler`. Only `GET /foo` answers JSON (`MessageBody`). Errors carry no JSON: 404/405 come from `http.ServeMux` defaults (there is deliberately no catch-all route), a `Fooer` error becomes `http.Error` 500 and is logged, never echoed.

## Conventions

- Branches: `devel` is the default branch and the target of feature PRs; `main` is release-only and protected (PRs from `devel`, CI must pass). Never push to `main` directly.
- Tests use testify (`require` when the test can't continue, `assert` otherwise), call `t.Parallel()`, and never bind fixed ports.
- Imports are grouped std / third-party / `github.com/peczenyj/foo` (gci); `make fmt` fixes them.
- `CHANGELOG.md` is append-only: add bullets at the end, under `## [Unreleased]` (the last section). `.gitattributes` sets `merge=union` on it.
- `docs/superpowers/` (design specs, plans) and hidden `.*.md` notes are git-ignored: local working docs, never committed.
