# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

- `make test`: all tests with `-race`. Single test: `go test -race -run '^TestAPI$/should_reject_POST' ./internal/api/`
- `make lint`: `go vet` + golangci-lint linters. `make fmt` rewrites files (gofumpt + gci); `make fmt-check` only reports and is what CI runs.
- `make hurl`: builds `bin/foo`, starts it on a free port (`-port=0`, the bound address is read from the `"msg":"listening"` JSON log line), runs `test/hurl/*.hurl`, stops it. `scripts/hurl.sh <base-url>` targets a running server.
- `make mocks`: regenerate `internal/api/mocks` with `go tool mockery` (pinned in `go.mod`, configured in `.mockery.yml`). Run it after changing `foo.Fooer`; CI fails on stale mocks. Never hand-edit the mocks.
- `make vendor`: tidy + vendor + verify. `vendor/` is git-ignored and must never be committed; while it exists locally, Go builds from it.
- `make build-debug`: `bin/foo-debug` keeps DWARF and the symbol table, with `-N -l` (no optimizations or inlining) and no `-trimpath`, for `dlv exec bin/foo-debug`. `make build` is the stripped release binary.
- `make run ARGS="-log-level=debug -log-format=json"`, `make docker`.

## Architecture

`cmd/server/main.go` `run()` → `internal/config.Parse` (flags, with `FOO_*` env vars as defaults; flag > env > default; every invalid value reported, exit 2) → `cfg.NewLogger` (slog text/json) → `api.New(foo.Bar{})` → `RunServer` (listens first, logs the bound address, graceful shutdown on SIGINT/SIGTERM or ctx cancel).

- `internal/foo`: the `Fooer` interface (`Foo(ctx) (string, error)`) and the real implementation `Bar`.
- `internal/api`: the `http.Handler`. Only `GET /foo` answers JSON (`MessageBody`). Errors carry no JSON: 404/405 come from `http.ServeMux` defaults (there is deliberately no catch-all route), a `Fooer` error becomes `http.Error` 500 and is logged, never echoed.

The layout and conventions mirror `github.com/peczenyj/go-fizzbuzz`; when in doubt, follow it.

## Conventions

- Branches: `devel` is the default branch and the target of feature PRs; `main` is release-only and protected (PRs from `devel`, CI must pass). Never push to `main` directly.
- Tests use testify (`require` when the test can't continue, `assert` otherwise), call `t.Parallel()`, and never bind fixed ports.
- Imports are grouped std / third-party / `github.com/peczenyj/foo` (gci); `make fmt` fixes them.
- `CHANGELOG.md` is append-only: add bullets at the end, under `## [Unreleased]` (the last section). `.gitattributes` sets `merge=union` on it.
- Design and plan docs live in `docs/superpowers/`.
