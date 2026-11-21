# Changelog

All notable changes to this project are documented here, and this project
adheres to [Semantic Versioning](https://semver.org/).

This file is append-only: every merge request adds its bullets at the end,
under `## [Unreleased]`, which is always the last section. Releases are listed
oldest first; to release, rename `[Unreleased]` to the version and date, then
open a new `## [Unreleased]` below it. `.gitattributes` merges this file with
`merge=union`, so two merge requests appending at the same time don't conflict.

## [Unreleased]

- `GET /foo` replies `{"message":"bar"}`; other paths and methods get the
  standard 404/405 status, with no JSON body.
- `GET /healthz` for liveness checks.
- `Fooer` interface, with a mockery-generated mock (`make mocks`) for tests.
- Host, port, log level (`debug` for debug mode) and log format (`json`) set by
  flag or `FOO_*` environment variable.
- hurl suite (`make hurl`), linters and formatters through golangci-lint
  (`make lint`, `make fmt`, `make fmt-check`), Docker image and CI.
- `make vendor` vendors the dependencies locally; `vendor/` is never committed.
- README badges: CI status, latest tag and Go version.
- Test coverage uploaded to Codecov from CI (Go 1.27 job), with a README badge.
- Branches: `devel` is the default branch, `main` is protected and holds releases; CI runs on both.
- Targets Go 1.27: `go.mod` requires it and CI tests only 1.27; the README badge reads the version from `go.mod`.
- `make build-debug` builds `bin/foo-debug` with debug symbols and without optimizations, for debuggers such as Delve.
- Coverage leaves out the generated mocks (`make cover`, CI and `codecov.yml`).
