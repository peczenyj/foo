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

## [0.2.0] - 2026-11-22

- Released images are published to `ghcr.io/peczenyj/foo` (the tag, and
  `latest`) for linux/amd64 and linux/arm64 when a GitHub Release is
  published; the Release workflow can also republish an existing tag by hand.
- mockery, gotestsum and benchstat are installed binaries, like golangci-lint,
  instead of `go tool`s in `go.mod`: `make install-tools` installs the pinned
  versions, each target checks the tools it runs, and `go.mod` drops their
  dependencies.
- Two lint profiles: full (`.golangci.yml`, what CI runs) and lite
  (`.golangci.lite.yml`, correctness linters only), selected with
  `LINT_PROFILE`; `make lint-profiles`, in `make ci`, checks that lite stays
  a subset of full.
- One project name: `NAME` in the Makefile and the module path in `go.mod`
  drive the binary paths, the image tag and labels, the coverage filter and
  gci's import grouping; environment variables share one `envPrefix`. The
  image's entrypoint is now `/server` instead of `/foo`. The README lists
  the few places that still name the project.
- `make rename MODULE=<module path> NAME=<name>` adapts the template to
  another module and name (Go imports, `go.mod`, mocks, `programName`,
  `envPrefix`, Dockerfile defaults, docs), for a new project copied with
  `gonew` or for an existing repository with its own `go.mod`; it handles
  `/vN` modules and modules without a dot (`widget`), and warns about any
  file that still names the old module.
- Fix: the Makefile runs its scripts with `bash`, so a copy made with
  `gonew` works: Go module zips keep no executable bits, and every script
  failed with "Permission denied" there.

## [Unreleased]

- Errors from `GET /foo` are RFC 9457 problem details
  (`application/problem+json`) instead of plain text: `500` with a generic
  `detail` (the error is logged), and `400` with only its `Detail` when a
  `Fooer` returns a `*foo.InputError`, however it is wrapped. `problemFor`
  is the single error-to-status mapping.
  The router's `404` and `405` keep their plain-text body.
