# foo service — design

Date: 2026-11-21

## Intent

A small Go HTTP service that serves as an example of good practices.
`GET /foo` replies `{"message":"bar"}`; anything else gets the standard HTTP
error status (404, 405, 500) with no JSON body.
The value is in the scaffolding around it: an interface-driven handler, a
generated mock, layered tests (unit with mock, integration with the real
implementation, black-box hurl), CLI/env configuration, structured logging,
lint, Docker and CI.

## Goals

- A `Fooer` interface with a method `Foo`, a real implementation, and a
  mockery-generated mock.
- Handler unit tests using the mock (including the error path).
- An in-process test of the whole HTTP stack with the real `Fooer`.
- A hurl suite run against the live binary: `/foo` → 200 `bar`, `/bar` → 404 error.
- Host and port configurable by flag and environment variable; debug log
  level and JSON log format switchable the same way.
- Makefile, golangci-lint config, Dockerfile and GitHub Actions CI.

## Non-goals

- OpenAPI spec and fuzzing (left for later).
- Any third-party runtime dependency.

## Anti-goals

- Inconsistent or unexplained patterns: this repo is meant to be read as an
  example.
- Tests that depend on fixed ports or each other (they must run with `-race`
  and in parallel).

## Approaches

- **A. Standard library — chosen.** stdlib `flag` with `FOO_*` env
  vars as defaults (flag > env > built-in default), `cmd/server` + `internal/*`
  layout, graceful shutdown. Zero runtime deps; costs ~150 lines
  of config code we own and test.
- **B. CLI library (kong, or cobra+viper).** Less config code via struct tags,
  but adds a runtime dependency.

## Design

### Layout

```
cmd/server/            main.go: run() → config, logger, server, graceful shutdown
internal/config/       Parse(name, args, getenv, output) (Config, error)
internal/foo/          Fooer interface + real implementation
internal/api/          API (http.Handler) routing /healthz, /foo, 404 fallback
internal/api/mocks/    mockery-generated MockFooer (committed)
test/hurl/             foo.hurl
.mockery.yml
```

### `internal/foo`

```go
type Fooer interface {
    Foo(ctx context.Context) (string, error)
}
```

The real implementation returns `"bar", nil`. The error return exists so the
handler's 500 path can be tested with the mock.

### `internal/api`

- `New(f foo.Fooer) *API` — panics on nil (wiring bug).
- Routes: `GET /healthz` → 200 empty; `GET /foo` → 200 `{"message":"bar"}`
  with `Content-Type: application/json`, via a `writeJSON` helper. JSON is
  only for this success case.
- Errors are the standard HTTP status with no JSON body: unknown path → 404
  and wrong method → 405 (with `Allow`), both left to `http.ServeMux`'s
  defaults; `Fooer` error → `http.Error(w, http.StatusText(500), 500)`, error
  logged, its text never sent to the client.

### `internal/config`

| Flag | Env | Default |
|------|-----|---------|
| `-host` | `FOO_HOST` | `""` (all interfaces) |
| `-port` | `FOO_PORT` | `8080` (validated 0–65535; 0 = any free port) |
| `-log-level` | `FOO_LOG_LEVEL` | `info` (debug, info, warn, error) |
| `-log-format` | `FOO_LOG_FORMAT` | `text` (text, json) |
| `-shutdown-timeout` | `FOO_SHUTDOWN_TIMEOUT` | `10s` |
| `-version` | — | print version and exit |

`Config.Addr()` returns `net.JoinHostPort(host, port)`. Invalid env vars and
flags are reported to stderr; `run` exits 2 on usage errors, 1 on server
failure, 0 otherwise.

### Tests

All Go tests use testify (`require` for checks the test can't continue
past, `assert` otherwise); it is a test-only dependency, also required by
mockery's testify template.

- `internal/api`: table tests with `mocks.MockFooer` (success, error → 500,
  404, 405, content type), via `httptest.NewRecorder`.
- `internal/api`: integration test with the real `foo` implementation behind
  `httptest.NewServer`, issuing real HTTP requests.
- `internal/config`: flag/env precedence and validation tables.
- `cmd/server`: `run()` tests for `-version`, `-h`, bad flags, start+cancel on
  port 0, address-in-use.
- `test/hurl/foo.hurl`: `GET /foo` → 200, `jsonpath "$.message" == "bar"`;
  `GET /bar` → 404; `POST /foo` → 405; `GET /healthz` → 200.

### Tooling

- **mockery v3**, pinned as a `tool` directive in `go.mod`, run with
  `go tool mockery` (`make mocks`). Config in `.mockery.yml`. Generated
  mocks are committed; CI checks they are up to date (`make mocks` + `git diff
  --exit-code`).
- **hurl** is an external binary. `make hurl` builds the server, starts it on
  a free port in the background, waits for `/healthz`, runs
  `hurl --test --variable base_url=…`, and stops the server. Script in
  `scripts/hurl.sh`.
- **Makefile** targets: `help build run test cover lint fmt
  tidy docker clean`, plus `mocks`, `hurl` and `fmt-check`. golangci-lint is
  the single entry point for both linters and formatters, driven by
  `.golangci.yml`:
  - `make lint` → `go vet ./...` + `golangci-lint run ./...` (linters only).
  - `make fmt` → `golangci-lint fmt ./...` (rewrites files with gofumpt + gci).
  - `make fmt-check` → `golangci-lint fmt --diff ./...` (fails if any file
    is unformatted; used in CI, changes nothing).
  - `GOLANGCI_LINT ?= golangci-lint` so a different binary/version can be
    passed (`make lint GOLANGCI_LINT=…`).
- **.golangci.yml**: golangci-lint v2, `standard` linters plus bodyclose,
  errorlint, gocritic, gocyclo, gosec, misspell, prealloc and revive;
  gofumpt and gci (prefix `github.com/peczenyj/foo`); mocks excluded.
- **Dockerfile**: two-stage build into distroless `static:nonroot`, binary `/foo`.
- **CI** (`.github/workflows/ci.yml`): lint (golangci-lint action, then
  `make fmt-check`), test (Go 1.26.x and 1.27.x,
  race + coverage), mocks-up-to-date check, govulncheck, a hurl job (installs
  hurl, runs `make hurl`), docker build + hurl smoke test against the
  container. No codecov upload or GHCR publish unless asked.
- `go.mod`: `module github.com/peczenyj/foo`, `go 1.26`.
- README: what the service is, how to run, configure and test it.

## Left to the builder

- Exact test names and table contents, log messages, README wording.
- Pinned versions for GitHub Actions (full commit SHAs).
