# AGENTS.md

This file provides guidance to AI coding agents (Claude Code, Codex, Gemini CLI, Cursor and others) when working with code in this repository. `CLAUDE.md` only imports it (`@AGENTS.md`): edit this file, not that one.

## Commands

- `make task`: run before committing. tidy → lint-fix → fmt → ci; it edits files. `make ci` is read-only (mod-check, fmt-check, lint, cover) and runs the checks of CI's lint and test jobs. Keep `ci` read-only: anything that fixes belongs in `task`. Workflow targets chain their steps as prerequisites, run left to right (`.NOTPARALLEL`); never call `$(MAKE)` from a recipe.
- `make test`: all tests with `-race`. Single test: `go test -race -run '^TestAPI$/should_reject_POST' ./internal/api/`.
- golangci-lint is the only tool to install: its version is pinned in the Makefile (`GOLANGCI_LINT_VERSION`), `.github/workflows/ci.yml` and `.gitlab-ci.yml` (keep the three in sync). `make check-tools` checks it (`scripts/check-tool.sh`: missing → error, other version → warning).
- `make lint`: `go vet` + golangci-lint (`.golangci.yml`: linters that find bugs, not style rules). `make fmt` rewrites files (gofumpt + gci); `make fmt-check` only reports and is what CI runs. A `//nolint` names the linter and gives a reason.
- `make build` (`bin/<NAME>`), `make run ARGS="-log-level=debug -log-format=json"`, `make docker`.
- CI: `.github/workflows/ci.yml` and `.gitlab-ci.yml` run the same make targets (lint, test, then a Docker build smoke-tested on `/healthz`); a check added to one goes in the other too.

## Architecture

`cmd/server/main.go` `run()` → `internal/config.Parse` (flags, with environment variables as defaults, `envPrefix` + setting; flag > env > default; every invalid value reported, exit 2) → `cfg.NewLogger` (slog text/json) → `api.New(foo.New(), logger, api.WithRecorder(metrics.Expvar()))` → `serveAll`: `RunServer` (listens first, logs `"listening"` with the bound address, graceful shutdown on SIGINT/SIGTERM or ctx cancel) and, when `cfg.AdminEnabled()`, `RunAdminServer` (`admin.New()`, logs `"admin listening"`; its in-flight requests are cancelled at shutdown, so a running profile ends early); if either fails, the other shuts down too.

- `internal/foo`: the `Fooer` interface (`Foo(ctx) (string, error)`) and the real implementation `*Bar`, built with `foo.New()` (a real implementation's dependencies become `New`'s parameters).
- `internal/metrics` and `internal/admin`: request metrics (`metrics.Recorder`, published with expvar by `metrics.Expvar()`, once per process) and the admin server (`GET /debug/vars`, `/debug/pprof/*`), off unless `-admin-port` is set (`-1` off, `0` any free port).
- `internal/api`: the `http.Handler`; it records every request (route pattern or `metrics.Unmatched`, status, duration) with the recorder given by `api.WithRecorder`. Register routes with `a.handle`. Handler errors are RFC 9457 problem details (`application/problem+json`, `api.Problem`): `problemFor` (`problem.go`) maps domain errors to statuses (`*foo.InputError` → 400 with its `Detail`, never `err.Error()`; anything else → 500 with a generic detail, the error logged, never echoed). Domain errors go through `a.writeError(w, err)`; never call `http.Error` in a handler. 404/405 come from `http.ServeMux` and stay plain text.

## Conventions

- Tests use testify (`require` when the test can't continue, `assert` otherwise; test-only, never in the binary), call `t.Parallel()`, and never bind fixed ports. Test layers: unit (`handler_test.go` and `metrics_test.go`, a hand-written fake `Fooer` in `fooer_test.go`: add a field there when the interface grows) and component (`component_test.go`, real `Fooer`, in-process HTTP).
- Secrets: environment-only settings held in `config.Secret` (never a flag, never a plain `string`), read with `readSecret` from the variable or from the file its `_FILE` variant names; `Secret` prints and logs as `REDACTED`; call `Reveal()` only where the value is sent. Never put a secret's value in an error or a log.
- Logging: `logger.Error(...)`, not `ErrorContext(ctx, ...)`.
- Imports are grouped std / third-party / this module; `make fmt` fixes them.
- One project name: `NAME` in the Makefile, `programName` (`cmd/server`) and `envPrefix` (`internal/config`; tests use the `config.Env*` constants), plus the module path in `go.mod`. Never write them anywhere else.
