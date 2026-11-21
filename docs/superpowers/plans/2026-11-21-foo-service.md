# foo service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go HTTP service where `GET /foo` returns `{"message":"bar"}`, built as a good-practices example: `Fooer` interface + mockery mock, layered tests, hurl suite, flag/env config, golangci-lint lint+fmt, Docker, CI.

**Architecture:** `cmd/server` wires `internal/config` → logger → `internal/api` (an `http.Handler` around a `ServeMux`) → `internal/foo` (the `Fooer`). stdlib only at runtime.

**Tech Stack:** Go (`go 1.26` directive; local toolchain 1.27.4), stdlib `net/http`, `flag`, `log/slog`; testify `assert`/`require`/`mock` (test-only: the assertion library for every test, and the base of mockery's template); mockery v3.9.1 as a `go tool`; hurl 8.0.1; golangci-lint v2 (local 2.15.0).

**Spec:** `docs/superpowers/specs/2026-11-21-foo-service-design.md`

## Global Constraints

- Module path `github.com/peczenyj/foo`; `go 1.26` in `go.mod` (raise only if `go get -tool` forces it, and then drop 1.26.x from the CI matrix).
- No third-party runtime dependency. testify and mockery are test/tool only.
- Env var prefix `FOO_`: `FOO_HOST`, `FOO_PORT`, `FOO_LOG_LEVEL`, `FOO_LOG_FORMAT`, `FOO_SHUTDOWN_TIMEOUT`. Precedence flag > env > default.
- Defaults: host `""`, port `8080`, log level `info`, log format `text`, shutdown timeout `10s`.
- JSON only on success: `GET /foo` → 200 `{"message":"bar"}`, `Content-Type: application/json`. Errors are the standard status with no JSON body: 404 and 405 from `http.ServeMux`'s defaults, 500 via `http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)`. Tests check error status codes only (plus `Allow` on 405), never error bodies.
- Exit codes: 0 ok, 1 server failure, 2 usage/config error.
- Every test calls `t.Parallel()`; no fixed ports in Go tests (use port 0 / `httptest`).
- Every test asserts with testify: `require` when the test cannot continue after a failure (errors, decoding, setup), `assert` otherwise. No hand-written `if … { t.Fatalf }` comparisons.
- Every commit message ends with the line `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- After every task: `make test lint fmt-check` passes.

## Review Focus

1. `POST /foo` (any non-GET/HEAD method on a known path) → 405 with `Allow: GET, HEAD`, not 404. Test in Task 2.
2. `/foo/` and `/FOO` → 404 (exact, case-sensitive match). Test in Task 2.
3. IPv6 host (`-host=::1`) → listen address `[::1]:8080`, not `::1:8080`. Test in Task 3.
4. Invalid `FOO_PORT` (`abc`, `70000`, `-1`) → exit 2, stderr names `FOO_PORT`, even when `-port` is also passed. Tests in Task 3.
5. A `Fooer` error's text never reaches the response body; the client gets a plain 500 and the error is logged. Test in Task 2.

---

### Task 1: Module, lint/format tooling, `Fooer`

**Files:**
- Create: `go.mod`, `go.sum`, `Makefile`, `.golangci.yml`, `internal/foo/foo.go`, `internal/foo/foo_test.go`

**Interfaces:**
- Produces:
  - `package foo`: `type Fooer interface { Foo(ctx context.Context) (string, error) }`
  - `type Bar struct{}` with `func (Bar) Foo(context.Context) (string, error)` returning `"bar", nil`; `var _ Fooer = Bar{}`.
  - Make targets `help build run test cover lint fmt fmt-check tidy clean` and variable `GOLANGCI_LINT ?= golangci-lint`.

- [ ] **Step 1: `go mod init github.com/peczenyj/foo`**, then set the `go` directive to `go 1.26`, then `go get github.com/stretchr/testify@latest`.

- [ ] **Step 2: Write `.golangci.yml`** — golangci-lint v2 config: `standard` linters plus bodyclose, errorlint, gocritic, gocyclo (15), gosec, misspell, prealloc, revive; formatters gofumpt and gci with sections standard, default, `prefix(github.com/peczenyj/foo)`; add `internal/api/mocks` to both `linters.exclusions.paths` and `formatters.exclusions.paths`.

- [ ] **Step 3: Write the `Makefile`** — self-documenting (`## ` comments listed by `help`), with `BIN := bin/foo`, version/revision ldflags and `GOLANGCI_LINT ?= golangci-lint`; targets `help build run test cover tidy clean` (Task 5 adds docker), plus:
  - `lint: ## go vet + golangci-lint linters` → `$(GO) vet ./...` then `$(GOLANGCI_LINT) run ./...`
  - `fmt: ## Format the code (gofumpt + gci)` → `$(GOLANGCI_LINT) fmt ./...`
  - `fmt-check: ## Fail if any file is not formatted` → `$(GOLANGCI_LINT) fmt --diff ./...`
  - widen the `help` awk column to `%-10s` so `fmt-check` aligns.

- [ ] **Step 4: Write the failing test** `internal/foo/foo_test.go` (package `foo_test`):

```go
func TestBar_Foo(t *testing.T) {
	t.Parallel()
	got, err := foo.Bar{}.Foo(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "bar", got)
}
```

- [ ] **Step 5: Run** `go test ./internal/foo/` — Expected: FAIL (`undefined: foo.Bar`).

- [ ] **Step 6: Implement** `Fooer` and `Bar` in `internal/foo/foo.go` with a package doc comment.

- [ ] **Step 7: Run** `make test lint fmt-check` — Expected: all pass, `make help` lists every target.

- [ ] **Step 8: Commit** `feat: add module, lint tooling and Fooer` (add go.mod, go.sum, Makefile, .golangci.yml, internal/foo).

---

### Task 2: mockery mock and the HTTP API

**Files:**
- Create: `.mockery.yml`, `internal/api/handler.go`, `internal/api/handler_test.go`, `internal/api/integration_test.go`, `internal/api/helpers_test.go`, `internal/api/mocks/mock_fooer.go` (generated)
- Modify: `go.mod`/`go.sum` (mockery tool), `Makefile` (add `mocks`)

**Interfaces:**
- Consumes: `foo.Fooer`, `foo.Bar` (Task 1).
- Produces:
  - `package api`: `func New(f foo.Fooer) *API` (panics `"api.New: nil Fooer"` on nil); `func (a *API) ServeHTTP(http.ResponseWriter, *http.Request)`.
  - `type MessageBody struct { Message string \`json:"message"\` }` (no error body type).
  - `package mocks`: `MockFooer`, `NewMockFooer(t)`, `EXPECT().Foo(...)` (mockery testify template).
  - Make target `mocks: ## Regenerate mocks with mockery` → `$(GO) tool mockery`.

- [ ] **Step 1: Add the tool** — `go get -tool github.com/vektra/mockery/v3@v3.9.1`.

- [ ] **Step 2: Write `.mockery.yml`:**

```yaml
all: false
dir: internal/api/mocks
filename: mock_fooer.go
pkgname: mocks
structname: Mock{{.InterfaceName}}
template: testify
force-file-write: true
formatter: goimports
packages:
  github.com/peczenyj/foo/internal/foo:
    interfaces:
      Fooer: {}
```

- [ ] **Step 3: Add the `mocks` Make target, run `make mocks`, then `go mod tidy`** — Expected: `internal/api/mocks/mock_fooer.go` exists with a `// Code generated by mockery` header and imports `github.com/stretchr/testify/mock`.

- [ ] **Step 4: Write `helpers_test.go`** — `doRequest(t, handler, method, target) (status int, body []byte, headers http.Header)` and `decodeJSON[T](t, body) T`, using `httptest.NewRecorder` and `require.NoError` for read and decode errors.

- [ ] **Step 5: Write the failing unit tests** in `handler_test.go` (package `api_test`), table-driven, each case builds its own `mocks.NewMockFooer(t)`:

| label | setup | method, target | status | body | extra |
|---|---|---|---|---|---|
| should reply the Fooer message | `EXPECT().Foo(mock.Anything).Return("bar", nil).Once()` | `GET /foo` | 200 | `MessageBody{"bar"}` | content type `application/json` |
| should reply whatever the Fooer returns | `Return("baz", nil)` | `GET /foo` | 200 | `MessageBody{"baz"}` | proves the handler doesn't hardcode |
| should hide the Fooer error | `Return("", errors.New("secret db failure"))` | `GET /foo` | 500 | not checked | body does not contain `secret` |
| should return 404 for an unknown path | none | `GET /bar` | 404 | not checked | |
| should return 404 for a trailing slash | none | `GET /foo/` | 404 | not checked | |
| should be case sensitive | none | `GET /FOO` | 404 | not checked | |
| should reject POST /foo | none | `POST /foo` | 405 | not checked | `Allow: GET, HEAD` |
| should reject DELETE /healthz | none | `DELETE /healthz` | 405 | not checked | `Allow: GET, HEAD` |
| should report healthy | none | `GET /healthz` | 200 | empty | |

Assertions: `assert.Equal` on status; for 200 `/foo` cases also decoded body and `assert.Equal(t, "application/json", headers.Get("Content-Type"))`; for error cases `assert.NotEqual(t, "application/json", headers.Get("Content-Type"))`; `assert.Equal(t, "GET, HEAD", headers.Get("Allow"))` on 405; `assert.NotContains(t, string(body), "secret")` on 500. Plus `TestNew_panics_on_nil_Fooer` with `assert.PanicsWithValue(t, "api.New: nil Fooer", …)`. Mock expectations are asserted automatically by `NewMockFooer(t)` cleanup, so a case with no setup fails if `Foo` is called.

- [ ] **Step 6: Write the failing integration test** `integration_test.go`: `TestAPI_with_real_Fooer` — `srv := httptest.NewServer(api.New(foo.Bar{}))`, `t.Cleanup(srv.Close)`; `require.NoError` on each request; `http.Get(srv.URL + "/foo")` → 200, `Content-Type: application/json`, decoded `MessageBody{"bar"}`; `http.Get(srv.URL + "/bar")` → 404. Close response bodies (bodyclose lint).

- [ ] **Step 7: Run** `go test ./internal/api/` — Expected: FAIL (`undefined: api.New`).

- [ ] **Step 8: Implement `internal/api/handler.go`.** A `ServeMux` with only two patterns:
  - `GET /healthz` → 200 empty.
  - `GET /foo` → calls `Foo(r.Context())`; on success `writeJSON(w, 200, &MessageBody{Message: msg})` (`writeJSON` sets the content type, writes the status, encodes, logs encoding errors); on error `slog.Error(..., slog.Any("error", err))` then `http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)`.
  - No catch-all and no custom 404/405 handlers: the mux's defaults produce 404 and 405 (with `Allow`).

- [ ] **Step 9: Run** `go test -race ./internal/api/` — Expected: PASS. Then `make test lint fmt-check` — PASS.

- [ ] **Step 10: Commit** `feat: add HTTP API with mockery-tested Fooer`.

---

### Task 3: Configuration from flags and env

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`

**Interfaces:**
- Produces (`package config`):
  - consts `EnvHost = "FOO_HOST"`, `EnvPort = "FOO_PORT"`, `EnvLogLevel = "FOO_LOG_LEVEL"`, `EnvLogFormat = "FOO_LOG_FORMAT"`, `EnvShutdownTimeout = "FOO_SHUTDOWN_TIMEOUT"`; `LogFormatText = "text"`, `LogFormatJSON = "json"`; `var ErrInvalid = errors.New("invalid configuration")`.
  - `type Config struct { Host string; Port int; LogLevel slog.Level; LogFormat string; ShutdownTimeout time.Duration; ShowVersion bool }`
  - `func Default() Config`; `func Parse(name string, args []string, getenv func(string) string, output io.Writer) (Config, error)`; `func (c Config) Addr() string` (`net.JoinHostPort(c.Host, strconv.Itoa(c.Port))`); `func (c Config) NewLogger(w io.Writer) *slog.Logger`.
  - Flags: `-host`, `-port`, `-log-level`, `-log-format`, `-shutdown-timeout`, `-version`.

- [ ] **Step 1: Write the failing tests** in `config_test.go` (package `config_test`, `getenv` built from a `map[string]string`):
  - `TestParse_defaults`: no args, no env → `Default()`, and `Addr() == ":8080"`.
  - `TestParse` table, each asserting the resulting `Config` field(s):
    - env only: `FOO_HOST=127.0.0.1`, `FOO_PORT=9090` → `Addr() == "127.0.0.1:9090"`.
    - flag beats env: env `FOO_PORT=9090`, args `-port=7070` → `Port == 7070`.
    - debug: `-log-level=debug` → `slog.LevelDebug`; env `FOO_LOG_LEVEL=debug` → same.
    - json: `-log-format=json` → `LogFormatJSON`; env `FOO_LOG_FORMAT=json` → same.
    - port 0 accepted → `Port == 0`.
    - IPv6: `-host=::1` → `Addr() == "[::1]:8080"`.
  - `TestParse_errors` table, each asserting `require.ErrorIs(t, err, config.ErrInvalid)` and `assert.Contains` on the output:
    - env `FOO_PORT=abc` → `FOO_PORT`; env `FOO_PORT=70000` → `FOO_PORT`; env `FOO_PORT=-1` → `FOO_PORT`.
    - env `FOO_PORT=abc` with `-port=7070` → still fails, output contains `FOO_PORT`.
    - `-port=65536` → `port 65536`; `-log-format=xml` → `log format "xml"`; `FOO_LOG_LEVEL=loud` → `FOO_LOG_LEVEL`; `-shutdown-timeout=0s` → `shutdown timeout`; extra arg `foo` → `unexpected arguments`.
  - `TestParse_help`: `-h` → `require.ErrorIs(t, err, flag.ErrHelp)`, output contains `-port`.
  - `TestParse_version_skips_flag_validation`: `-version -port=70000` → `require.NoError`, `assert.True(t, cfg.ShowVersion)` (`-version` skips flag validation).
  - `TestNewLogger`: JSON format writes a line starting `{`; debug level enables `Debug`; info level drops it.

- [ ] **Step 2: Run** `go test ./internal/config/` — Expected: FAIL (`undefined: config.Parse`).

- [ ] **Step 3: Implement `config.go`** with `readEnv` (generic `setIfValid`/`checked` helpers; an invalid value keeps the default and is reported), `validate` reporting every error, and `NewLogger`. Port check: `0 <= port <= 65535`, error message `must be between 0 and 65535`; validate message prefix `port %d`.

- [ ] **Step 4: Run** `go test -race ./internal/config/` then `make test lint fmt-check` — Expected: PASS.

- [ ] **Step 5: Commit** `feat: read host, port and logging from flags and env`.

---

### Task 4: Server entrypoint and hurl suite

**Files:**
- Create: `cmd/server/main.go`, `cmd/server/main_test.go`, `test/hurl/foo.hurl`, `scripts/hurl.sh`
- Modify: `Makefile` (add `hurl`)

**Interfaces:**
- Consumes: `config.Parse`, `config.Config.Addr`, `config.Config.NewLogger`, `api.New`, `foo.Bar`.
- Produces: binary `bin/foo`; `main.version`, `main.revision` ldflags vars; `-version` prints `foo <version> (revision <revision>)`; startup log `msg="listening"` with attribute `addr` = the real bound address; Make target `hurl: ## Run the hurl suite against a fresh server` → `scripts/hurl.sh`.

- [ ] **Step 1: Write the failing tests** `main_test.go`, table-driven on `run(ctx, args, getenv, stdout, stderr) int`, with testify (`assert.Equal` on exit code, `assert.Equal`/`assert.Contains` on stdout/stderr, `require.NoError` on `net.Listen`):
  - `TestRun` table: `-version` → exit 0, stdout `foo dev (revision unknown)\n`; `-h` → 0, stderr `Usage of foo`; `-nope` → 2, `flag provided but not defined`; `-port=70000` → 2, `invalid configuration`; env `FOO_PORT=abc` (via getenv) → 2, `FOO_PORT`.
  - `TestRun_serves_until_cancelled`: cancelled ctx, args `-host=127.0.0.1 -port=0 -log-format=json` → 0; stderr contains `"msg":"listening"` and `"addr":"127.0.0.1:`.
  - `TestRun_fails_if_the_address_is_in_use`: occupy a port with `net.Listen`, pass its host/port → 1, stderr `address already in use`.

- [ ] **Step 2: Run** `go test ./cmd/server/` — Expected: FAIL.

- [ ] **Step 3: Implement `main.go`** — `run` (parse config, logger, signal context, exit codes), `RunServer` and `prepareServerShutdown` (graceful shutdown within `ShutdownTimeout`), with `programName = "foo"`; `RunServer` does `ln, err := net.Listen("tcp", cfg.Addr())`, logs `slog.Info("listening", slog.String("addr", ln.Addr().String()))`, then `server.Serve(ln)`. Log `application start` with version, revision, log level and format.

- [ ] **Step 4: Run** `go test -race ./cmd/server/` — Expected: PASS.

- [ ] **Step 5: Write `test/hurl/foo.hurl`:**

```hurl
GET {{base_url}}/foo
HTTP 200
[Asserts]
header "Content-Type" == "application/json"
jsonpath "$.message" == "bar"

GET {{base_url}}/bar
HTTP 404

POST {{base_url}}/foo
HTTP 405
[Asserts]
header "Allow" == "GET, HEAD"

GET {{base_url}}/healthz
HTTP 200
```

- [ ] **Step 6: Write `scripts/hurl.sh`** (`set -euo pipefail`, executable). Usage `scripts/hurl.sh [base-url]`:
  - With a base URL: run `hurl --test --variable base_url=<url> test/hurl/*.hurl` and exit with its status.
  - Without: fail early if `hurl` is missing; `make build`; start `bin/foo -host=127.0.0.1 -port=0 -log-format=json` in the background with stderr to a temp file; `trap` kills it and removes the temp file on EXIT; poll the log (≤ 10 s) for the `"msg":"listening"` line and extract `addr` with `sed`/`grep -o`; run hurl against `http://<addr>`.

- [ ] **Step 7: Add the `hurl` Make target and run `make hurl`** — Expected: `Success` for `test/hurl/foo.hurl` (4 requests), no `bin/foo` process left (`pgrep -f bin/foo` empty).

- [ ] **Step 8: Prove the suite can fail** — temporarily change `jsonpath "$.message" == "bar"` to `"baz"`, run `make hurl`, Expected: non-zero exit with an assert error; revert.

- [ ] **Step 9: Run** `make test lint fmt-check` — PASS.

- [ ] **Step 10: Commit** `feat: add server entrypoint and hurl suite`.

---

### Task 5: Docker, CI and docs

**Files:**
- Create: `Dockerfile`, `.dockerignore`, `.github/workflows/ci.yml`, `.github/dependabot.yml`, `README.md`
- Modify: `Makefile` (add `docker`), `CLAUDE.md` (untracked file from `/init`; replace with real content and commit)

**Interfaces:**
- Consumes: Make targets `mocks`, `hurl`, `fmt-check`; `scripts/hurl.sh <base-url>`; `-version` output.

- [ ] **Step 1: Dockerfile + .dockerignore** — two-stage build (golang builder with module and build caches, `CGO_ENABLED=0 -trimpath`, version ldflags) into `gcr.io/distroless/static-debian12:nonroot` with OCI labels; binary `/out/foo` → `/foo`, `ENTRYPOINT ["/foo"]`, title `foo`, description `Example Go HTTP service: GET /foo replies bar`, source `https://github.com/peczenyj/foo`, license `MIT`. Note: the binary's default host `""` listens on all interfaces, which the container needs. Add `docker: ## Build the Docker image` to the Makefile (tag `foo:$(VERSION)`).

- [ ] **Step 2: Run** `make docker && docker run --rm foo:$(git describe --tags --always --dirty) -version` — Expected: `foo <version> (revision <sha>)`. Then `docker run -d --rm --name foo-smoke -p 18080:8080 foo:<tag>`, `scripts/hurl.sh http://localhost:18080` — Expected: Success; `docker stop foo-smoke`.

- [ ] **Step 3: `.github/workflows/ci.yml`** — actions pinned to full commit SHAs, `permissions: contents: read`, per-ref concurrency, with these jobs:
  - `lint`: golangci-lint action (`version: v2.15.0`), then `make fmt-check`. No OpenAPI step.
  - `test`: matrix `1.26.x`, `1.27.x`; vet; `go test -race -covermode=atomic -coverprofile=coverage.txt ./...`; coverage summary. No benchmarks, no codecov.
  - `mocks`: `make mocks` then `git diff --exit-code`.
  - `vulncheck`: `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` on Go stable.
  - `hurl`: install hurl 8.0.1 (`curl -fsSLO https://github.com/Orange-OpenSource/hurl/releases/download/8.0.1/hurl_8.0.1_amd64.deb && sudo apt-get install -y ./hurl_8.0.1_amd64.deb`), then `make hurl`.
  - `docker` (needs lint, test, hurl): build the image with VERSION/REVISION/CREATED build args, check `-version` output starts with `foo `, run container on 8080, wait for `/healthz`, install hurl as above, `scripts/hurl.sh http://localhost:8080`; container logs on `always()`.
  - No `publish` job.

- [ ] **Step 4: Validate the workflow** — `go run github.com/rhysd/actionlint/cmd/actionlint@latest` — Expected: no output.

- [ ] **Step 5: dependabot.yml** — gomod + github-actions weekly.

- [ ] **Step 6: README.md** — what it is; endpoints and responses (JSON on `GET /foo` success; plain 404/405/500 otherwise); config table from the spec; `make` targets (paste `make help`); how tests are layered (mock unit tests, real-`Fooer` integration test, hurl); how to regenerate mocks.

- [ ] **Step 7: CLAUDE.md** — keep the required header; replace the "no code yet" content with: the `make` commands incl. single-test form `go test -race -run '^TestName$' ./internal/api/`, `make mocks` after changing `Fooer`, `make fmt` vs `make fmt-check`, `make hurl`; a short architecture paragraph (config → logger → api → Fooer; routes live in `internal/api/handler.go`, JSON only on success and 404/405 left to `http.ServeMux` defaults; mocks are generated, never hand-edited).

- [ ] **Step 8: Run** `make test lint fmt-check hurl` — Expected: all pass.

- [ ] **Step 9: Commit** `chore: add Docker, CI, README and CLAUDE.md`.
