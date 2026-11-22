# foo

[![CI](https://github.com/peczenyj/foo/actions/workflows/ci.yml/badge.svg)](https://github.com/peczenyj/foo/actions/workflows/ci.yml)
[![Latest tag](https://img.shields.io/github/v/tag/peczenyj/foo?sort=semver)](https://github.com/peczenyj/foo/tags)
[![Go 1.27](https://img.shields.io/badge/go-1.27-00ADD8?logo=go)](https://go.dev/doc/go1.27)
[![codecov](https://codecov.io/gh/peczenyj/foo/branch/devel/graph/badge.svg)](https://app.codecov.io/gh/peczenyj/foo/tree/devel)

A small Go HTTP service written as an example of good practices: an
interface-driven handler, a generated mock, layered tests, configuration from
flags and environment variables, structured logs, linters and formatters,
Docker and CI.

## API

| Request | Response |
|---------|----------|
| `GET /foo` | `200` `{"message":"bar"}`, `Content-Type: application/json` |
| `GET /healthz` | `200`, empty body |
| any other path | `404` |
| another method on `/foo` or `/healthz` | `405`, with `Allow: GET, HEAD` |
| the `Fooer` fails | `500`; the error is logged, never sent |

JSON is only for the success case: errors are the standard status, with Go's
default plain-text body.

## Running

```sh
make run                                   # listens on :8080
make run ARGS="-port=9090 -log-level=debug -log-format=json"
FOO_PORT=9090 FOO_LOG_FORMAT=json make run
curl localhost:8080/foo                    # {"message":"bar"}
```

## Configuration

Flags win over environment variables, which win over the defaults. Every
invalid value is reported at startup, and the server exits with status 2.

| Flag | Environment | Default | |
|------|-------------|---------|-|
| `-host` | `FOO_HOST` | empty | listen host; empty for all interfaces |
| `-port` | `FOO_PORT` | `8080` | 0 to 65535; 0 for any free port |
| `-log-level` | `FOO_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `-log-format` | `FOO_LOG_FORMAT` | `text` | `text` or `json` |
| `-shutdown-timeout` | `FOO_SHUTDOWN_TIMEOUT` | `10s` | grace period for in-flight requests |
| `-version` | | | print the version and exit |

## Development

```text
  help              Show available targets
  task              Before a commit: generate, tidy, fix, format, then run ci (edits files)
  ci                All checks, read-only: mod-check, fmt-check, lint, cover (same as GitHub CI)
  generate          go generate ./... + regenerate mocks
  go-generate       go generate ./...
  check-tools       Fail with install instructions if golangci-lint is missing
  build             Build the server binary into bin/
  build-debug       Build bin/foo-debug with debug symbols, no optimizations (for dlv)
  run               Run the server locally (flags via ARGS, e.g. ARGS=-log-level=debug)
  test              Run all tests with the race detector (gotestsum; GOTESTSUM_FORMAT=testname lists each test)
  cover             Run tests with a coverage summary (mocks excluded)
  bench             Benchmarks (time, B/op, allocs/op) x BENCH_COUNT into bench.out, summarised by benchstat
  bench-compare     Benchmarks of BASE (default devel) vs the working tree, compared by benchstat
  bench-profile     CPU and memory profiles of BENCH in BENCH_PKG, for go tool pprof
  lint              go vet + golangci-lint linters
  lint-fix          go fix + golangci-lint --fix (edits files)
  fmt               Format the code (gofumpt + gci)
  fmt-check         Fail if any file is not formatted
  mod-check         Fail if go.mod/go.sum need tidying (read-only), then verify
  test-functional   Functional tests: the hurl suite against a fresh bin/foo (needs hurl)
  test-integration  Integration tests (TestIntegration_*, build tag integration; may need Docker)
  mocks             Regenerate mocks/ with mockery (.mockery.yaml)
  tidy              go mod tidy + verify
  vendor            go mod tidy + vendor + verify (vendor/ is git-ignored)
  docker            Build the Docker image
  clean             Remove build artefacts
```

Before committing, run `make task`: it regenerates code and mocks, tidies
`go.mod`, applies `go fix` and the linters' fixes, formats, then runs
`make ci`. `make ci` only checks (it never edits a file) and runs the checks
of GitHub CI's lint and test jobs. CI also runs govulncheck, the stale-mock
check, the hurl suite (`make test-functional`) and a smoke test of the Docker
image, which `make ci` leaves out.

Tools: Go 1.27 or later, [golangci-lint](https://golangci-lint.run/docs/welcome/install/)
v2.15.0 (the version CI pins), [hurl](https://hurl.dev/), Docker. The lint
and format targets run `make check-tools` first: it stops with the install
link if golangci-lint is missing, and warns if your version differs from
CI's. [mockery](https://vektra.github.io/mockery/) and
[gotestsum](https://github.com/gotestyourself/gotestsum) are pinned in
`go.mod` as tools and run with `go tool`; nothing to install. Tests run
through gotestsum: `make test GOTESTSUM_FORMAT=testname` lists every test.

### Tests

Each layer catches what the one before can't:

1. **Unit tests** (`internal/api/handler_test.go`, `make test`) drive the
   handler with a mockery mock of `Fooer`, including the failure path.
2. **Component tests** (`internal/api/component_test.go`, `make test`) run the
   API with the real `Fooer` behind a real HTTP server, in-process, through
   its public HTTP interface. Nothing outside the process is involved.
3. **Integration tests** (`make test-integration`) are reserved for tests
   against real external dependencies (a database, a cache, another
   service), typically started in Docker by testcontainers-go or dockertest.
   They live in files with `//go:build integration`, are named
   `TestIntegration_*`, and never run with `make test`. foo has no external
   dependency yet: `internal/api/integration_test.go` only shows the
   convention and skips.
4. **Functional tests** (`test/hurl/foo.hurl`, `make test-functional`) test
   the built binary from the outside with [hurl](https://hurl.dev/): the
   target starts it on a free port, runs the suite and stops it;
   `scripts/hurl.sh http://host:port` tests a server that is already running.

All Go tests use [testify](https://github.com/stretchr/testify).

### Benchmarks

`BenchmarkAPI_GetFoo` measures the `GET /foo` hot path: routing, the real
`Fooer` and JSON encoding. Benchmarks are read by people, not judged by CI:

- `make bench` runs every benchmark `BENCH_COUNT` times (6) with `-benchmem`
  and summarises time, bytes and allocations per operation with
  [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) (a
  `go tool`). A single run is noise; the `±` tells you how much to trust
  the number.
- `make bench-compare` answers "did my change make it faster or slower?": it
  benchmarks `BASE` (default `devel`, checked out in a temporary git
  worktree) and your working tree, uncommitted changes included, then
  benchstat prints the difference and whether it is significant (`~` means
  no significant change). `BENCH=GetFoo` narrows it to one benchmark.
- `make bench-profile` writes CPU and memory profiles for `go tool pprof`.

Neither CI nor `make task` runs benchmarks: shared runners are too noisy to
judge timings. They still compile with every `make test`, since `go test`
builds them.

After changing the `Fooer` interface, run `make mocks` and commit the result:
CI fails when the mocks are stale.

### Branches

`devel` is the default branch: merge requests target it. `main` holds
releases only; it is protected and changes reach it through a pull request
from `devel`. Release tags (`v*`) are cut on `main`.

### Changelog

`CHANGELOG.md` is append-only: add your bullets at the end of the file, under
`## [Unreleased]`.
