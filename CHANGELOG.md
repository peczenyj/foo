# Changelog

All notable changes to this project are documented here, and this project
adheres to [Semantic Versioning](https://semver.org/).

Append-only: each merge request adds its bullets at the end of
`## [Unreleased]`, always the last section (`.gitattributes` merges this file
with `merge=union`). To release, rename `[Unreleased]` to the version and
date, then open a new `## [Unreleased]` below it.

## [Unreleased]

- `GET /foo` replies `{"message":"bar"}` and `GET /healthz` answers liveness
  checks; errors are the standard 404/405/500 status, with no body.
- Configuration by flag or `FOO_*` environment variable: host, port, log
  level, log format (text or JSON) and shutdown timeout.
- `Fooer` interface, its real implementation (`foo.New()`) and mockery mocks
  in `mocks/`.
- Tests with testify, run through gotestsum: unit (mocked), component,
  integration (`make test-integration`, build tag `integration`) and
  functional (hurl, `make test-functional`).
- Benchmarks for people to read and compare: `make bench`,
  `make bench-compare` and `make bench-profile`; CI never runs them.
- Makefile: `make task` before committing, `make ci` for the read-only checks
  GitHub CI runs; golangci-lint (61 linters, gofumpt, gci) is installed by
  each developer (`make check-tools`); mockery, gotestsum and benchstat are
  pinned in `go.mod` as tools. Also `make build-debug` and `make vendor`.
- Distroless Docker image.
- GitHub CI on Go 1.27: lint, tests with coverage on Codecov, stale-mock
  check, govulncheck, and hurl against the binary and the container.
- Branches: `devel` (default) and protected, release-only `main`.
- README badges, a wiki, and `AGENTS.md` for AI coding agents.
