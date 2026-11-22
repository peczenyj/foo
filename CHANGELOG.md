# Changelog

All notable changes to this project are documented here, and this project
adheres to [Semantic Versioning](https://semver.org/).

Append-only: each merge request adds its bullets at the end of
`## [Unreleased]`, always the last section (`.gitattributes` merges this file
with `merge=union`). To release, rename `[Unreleased]` to the version and
date, then open a new `## [Unreleased]` below it.

## [0.1.0] - 2026-11-22

- `GET /foo` replies `{"message":"bar"}` and `GET /healthz` answers liveness
  checks; errors are the standard 404/405/500 status with `net/http`'s
  plain-text body, never JSON.
- Configuration by flag or `FOO_*` environment variable: host, port, log
  level, log format (text or JSON) and shutdown timeout. A host with a port
  (`localhost:8080`) or in brackets (`[::1]`) is a usage error (exit 2), and
  invalid flags are reported next to unexpected arguments.
- Graceful shutdown on SIGINT or SIGTERM; a second signal stops a hanging
  shutdown.
- Handler panics and accept errors from `net/http` are logged at error level,
  so `-log-level=warn` or `error` never hides them.
- `Fooer` interface, its real implementation (`foo.New()`) and mockery mocks
  in `mocks/`.
- Tests with testify, run through gotestsum: unit (mocked), component,
  integration (`make test-integration`, build tag `integration`) and
  functional (hurl, `make test-functional`, which builds `bin/foo` first and
  works whatever `FOO_LOG_LEVEL` is).
- Benchmarks for people to read and compare: `make bench`,
  `make bench-compare` and `make bench-profile`; CI never runs them.
- Makefile: `make task` before committing, `make ci` for the read-only checks
  GitHub CI runs; golangci-lint (64 linters, gofumpt, gci) is installed by
  each developer (`make check-tools`); mockery, gotestsum and benchstat are
  pinned in `go.mod` as tools. Also `make build-debug` and `make vendor`.
- Distroless Docker image; `.dockerignore` keeps `.env*`, `.claude` and local
  notes out of the build context.
- GitHub CI on Go 1.27: lint, tests with coverage on Codecov, stale-mock
  check, govulncheck, and hurl against the binary and the container.
- Branches: `devel` (default) and protected, release-only `main`.
- README badges, a wiki, and `AGENTS.md` for AI coding agents.

## [Unreleased]

- Released images are published to `ghcr.io/peczenyj/foo` (the tag, and
  `latest`) for linux/amd64 and linux/arm64 when a GitHub Release is
  published; the Release workflow can also republish an existing tag by hand.
