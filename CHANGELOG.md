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
- Functional tests with hurl (`make test-functional`), linters and formatters through golangci-lint
  (`make lint`, `make fmt`, `make fmt-check`), Docker image and CI.
- `make vendor` vendors the dependencies locally; `vendor/` is never committed.
- README badges: CI status, latest tag and Go version.
- Test coverage uploaded to Codecov from CI (Go 1.27 job), with a README badge.
- Branches: `devel` is the default branch, `main` is protected and holds releases; CI runs on both.
- Targets Go 1.27: `go.mod` requires it and CI tests only 1.27; the README badge shows it.
- `make build-debug` builds `bin/foo-debug` with debug symbols and without optimizations, for debuggers such as Delve.
- Coverage leaves out the generated mocks (`make cover`, CI and `codecov.yml`).
- `make task` (generate, tidy, fix, format, then ci) to run before committing; `make ci` runs every check read-only, as GitHub CI does; plus `make generate`, `make lint-fix` and `make mod-check`. CI now checks that go.mod/go.sum are tidy.
- `make check-tools`: the lint and format targets stop with the install link if golangci-lint is missing, and warn when its version differs from the one CI pins (2.15.0).
- Tests run through gotestsum (pinned in `go.mod` as a tool): `make test` and `make cover` use it, and CI uses its `github-actions` format.
- `BenchmarkAPI_GetFoo` and `make bench` (benchstat summary of time, B/op and allocs/op), and `make bench-profile`; neither `make task`, `make ci` nor CI runs benchmarks.
- `make bench-compare BASE=<ref>` compares benchmarks before and after a change with benchstat.
- Test layers named for what they test: the in-process test with the real `Fooer` is a component test (`component_test.go`); `make test-integration` runs `//go:build integration` tests (an example that skips for now), and the hurl suite runs with `make test-functional`.
- `.gitignore` covers editors, backups, build directories, env files, hidden notes, working docs (`docs/superpowers/`) and the local files of AI coding tools.
- Mocks live in the root `mocks/` folder, one package per mocked package (`foomock.Fooer`), configured in `.mockery.yaml`.
- Agent instructions live in `AGENTS.md`, shared by AI coding tools; `CLAUDE.md` only imports it (`@AGENTS.md`).
- Stricter linting: 61 linters on top of the standard set, grouped by purpose (HTTP and context, errors, slog, testify, tests, modern Go, style), with line length, magic numbers, function length, blank-line style and interface parameter names. Config errors are now sentinel errors, so every invalid value reads `must be ...`.
