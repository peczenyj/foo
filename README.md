# foo

[![CI](https://github.com/peczenyj/foo/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/peczenyj/foo/actions/workflows/ci.yml)
[![Latest tag](https://img.shields.io/github/v/tag/peczenyj/foo?sort=semver)](https://github.com/peczenyj/foo/tags)
[![Go 1.27](https://img.shields.io/badge/go-1.27-00ADD8?logo=go)](https://go.dev/doc/go1.27)
[![codecov](https://codecov.io/gh/peczenyj/foo/graph/badge.svg)](https://codecov.io/gh/peczenyj/foo)

A small Go HTTP service written as an example of good practices: an
interface-driven handler, a generated mock, layered tests, configuration from
flags and environment variables, structured logs, linters and formatters,
Docker and CI. Its layout follows
[go-fizzbuzz](https://github.com/peczenyj/go-fizzbuzz).

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
  help       Show available targets
  build      Build the server binary into bin/
  run        Run the server locally (flags via ARGS, e.g. ARGS=-log-level=debug)
  test       Run all tests with the race detector
  cover      Run tests with a coverage summary
  lint       go vet + golangci-lint linters
  fmt        Format the code (gofumpt + gci)
  fmt-check  Fail if any file is not formatted
  hurl       Run the hurl suite against a fresh server (needs hurl)
  mocks      Regenerate mocks with mockery (.mockery.yml)
  tidy       go mod tidy + verify
  vendor     go mod tidy + vendor + verify (vendor/ is git-ignored)
  docker     Build the Docker image
  clean      Remove build artefacts
```

Tools: Go 1.26 or later, [golangci-lint](https://golangci-lint.run/) v2,
[hurl](https://hurl.dev/), Docker. mockery is pinned in `go.mod` as a tool
and runs with `go tool mockery`; nothing to install.

### Tests

Three layers, each catching what the one before can't:

1. **Unit tests** (`internal/api/handler_test.go`) drive the handler with a
   mockery mock of `Fooer`, including the failure path.
2. **Integration test** (`internal/api/integration_test.go`) runs the API with
   the real `Fooer` behind a real HTTP server.
3. **hurl** (`test/hurl/foo.hurl`) tests the built binary from the outside:
   `make hurl` starts it on a free port, runs the suite and stops it;
   `scripts/hurl.sh http://host:port` tests a server that is already running.

All Go tests use [testify](https://github.com/stretchr/testify).

After changing the `Fooer` interface, run `make mocks` and commit the result:
CI fails when the mocks are stale.

### Changelog

`CHANGELOG.md` is append-only: add your bullets at the end of the file, under
`## [Unreleased]`.
