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
| any other path | `404`, plain text |
| another method on `/foo` or `/healthz` | `405`, plain text, with `Allow: GET, HEAD` |
| the `Fooer` rejects its input (`*foo.InputError`) | `400` problem; `detail` is the error's `Detail` |
| the `Fooer` fails | `500` problem; the error is logged, never sent |

Errors from `GET /foo` are [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457)
problem details, `Content-Type: application/problem+json`:

```json
{"type":"about:blank","title":"Internal Server Error","status":500,"detail":"the server could not complete the request"}
```

`type` is `about:blank` (the status says it all), `title` is the status
text. A `4xx` `detail` explains what the client got wrong; a `5xx` `detail`
is always that generic sentence, and the real error goes to the logs. One
function, `problemFor` in `internal/api/problem.go`, maps the errors a
handler gets from the domain to statuses (`a.writeError(w, err)`): to add a
case, define a typed error (or a sentinel) and add it there. The client sees
only the typed error's own field, never `err.Error()`, so context that
wrappers add on the way up stays in the logs. A handler that validates its
own request input (query parameters, a body) answers directly with
`a.writeProblem(w, http.StatusBadRequest, "<what is wrong>")`. Never call
`http.Error` in a handler. The router's own
`404` and `405` keep `net/http`'s plain-text body: a catch-all route to turn
them into problems would also turn every `405` into a `404`.

## OpenAPI

[`api/openapi.yaml`](api/openapi.yaml) describes the API (OpenAPI 3.0.3), so
anyone can generate a client for it, for example in TypeScript:

```sh
npx @hey-api/openapi-ts -i api/openapi.yaml -o client
```

The spec is the contract, written by hand and kept honest by tests:

- **`make openapi-lint`** ([vacuum](https://quobix.com/vacuum/), the
  recommended rules, warnings included) runs in `make ci` and GitHub CI. A
  rule that doesn't fit is switched off in `api/vacuum.yaml`, with a reason.
- **The contract test** (`internal/api/openapi_test.go`, kin-openapi, a
  test-only dependency: the server binary has no third-party module):
  - the routes `api.New` registers and the spec's operations are the same
    set, both ways;
  - every response of the handler and component tests is validated against
    the spec, an undocumented status included;
  - every response the spec documents is produced by a test, so the spec
    never promises what the service doesn't send.
- **Not described:** the router's own `404` and `405` (plain text) and
  `HEAD`.
- **Why 3.0.3, not 3.1:** kin-openapi and most client generators support 3.0
  best.
- `info.version` is the last release; the release pull request bumps it.

## Running

```sh
make run                                   # listens on :8080
make run ARGS="-port=9090 -log-level=debug -log-format=json"
FOO_PORT=9090 FOO_LOG_FORMAT=json make run
curl localhost:8080/foo                    # {"message":"bar"}
```

Released images (linux/amd64 and linux/arm64) are on the GitHub Container
Registry:

```sh
docker run --rm -p 8080:8080 ghcr.io/peczenyj/foo:v0.3.0   # or :latest
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

### Secrets

Secrets (API keys, tokens, passwords) are **environment-only** settings:
never flags, which show up in `ps` and in shell history. They are held in a
`config.Secret`, which prints, logs and encodes as `REDACTED` (any `fmt`
verb, `slog` text and JSON, `json.Marshal`), or `(unset)` when it holds no
value, so a config dump tells a missing secret from a configured one; only
`Reveal()` returns the value, in the one place that sends it. A trailing
newline (`\n` or `\r\n`, as `echo` or a file leave) is dropped; any other
space is kept, since it can be part of the secret. A missing or blank
required secret is a usage error at startup (exit 2, reported with every
other invalid value), never a failure on the first request.

foo needs no secret, so it declares none. Adding one takes three lines in
`internal/config/config.go`, plus a row in the table above with "env only"
in the flag column:

```go
EnvAPIKey = envPrefix + "API_KEY"                          // in the Env* constants
APIKey    Secret                                           // in Config
errs = append(errs, readSecret(getenv, EnvAPIKey, true, &c.APIKey)) // in readEnv
```

Then pass `cfg.APIKey.Reveal()` to whatever sends it.

Every secret can also come from a file, with the `_FILE` convention of
Docker and Kubernetes secrets: `FOO_API_KEY_FILE=/run/secrets/api_key`
instead of `FOO_API_KEY=<value>`. The value then stays out of the process
environment (`/proc/<pid>/environ`, `docker inspect`, child processes).

- The file is read once, at startup, up to 64 KiB, with the same trailing
  newline rule. Relative paths resolve against the working directory, and
  symlinks (Kubernetes mounts) are followed.
- These are usage errors (exit 2), even for an optional secret, since setting
  `_FILE` shows intent: both `FOO_API_KEY` and `FOO_API_KEY_FILE` set, a file
  that is missing or can't be read, a file larger than 64 KiB. A blank file
  counts as missing for a required secret. Errors name the variable and the
  path, never the content.
- The file is read to its end, so a named pipe or `/dev/stdin` holds
  startup until the writer closes it. That is also how to pipe a secret in
  without putting it in the environment:
  `printf %s "$KEY" | FOO_API_KEY_FILE=/dev/stdin ./bin/foo`.
- File permissions are left to the orchestrator: Kubernetes mounts secrets
  `0644` unless `defaultMode` says otherwise, and Docker often `0444`, so a
  permission warning would fire on most correct deployments.

## Development

```text
  help              Show available targets
  task              Before a commit: generate, tidy, fix, format, then run ci (edits files)
  ci                All checks, read-only: mod-check, fmt-check, lint, lint-profiles, openapi-lint, cover (same as GitHub CI)
  generate          go generate ./... + regenerate mocks
  go-generate       go generate ./...
  check-tools       Check every tool: missing → error with how to install it; other version → warning
  install-tools     go install the pinned mockery, gotestsum, benchstat and vacuum (TOOLS=gotestsum picks; needs network)
  build             Build the server binary, bin/<NAME>
  build-debug       Build bin/<NAME>-debug with debug symbols, no optimizations (for dlv)
  run               Run the server locally (flags via ARGS, e.g. ARGS=-log-level=debug)
  test              Run all tests with the race detector (gotestsum; GOTESTSUM_FORMAT=testname lists each test)
  cover             Run tests with a coverage summary (mocks excluded)
  bench             Benchmarks (time, B/op, allocs/op) x BENCH_COUNT into bench.out, summarised by benchstat
  bench-compare     Benchmarks of BASE (default devel) vs the working tree, compared by benchstat
  bench-profile     CPU and memory profiles of BENCH in BENCH_PKG, for go tool pprof
  lint              go vet + golangci-lint linters (LINT_PROFILE=full|lite)
  lint-profiles     Check the lite lint profile is a subset of full, and that the code passes it
  openapi-lint      Lint the OpenAPI spec, api/openapi.yaml (vacuum; warnings fail)
  lint-fix          go fix + golangci-lint --fix (edits files)
  fmt               Format the code (gofumpt + gci)
  fmt-check         Fail if any file is not formatted
  mod-check         Fail if go.mod/go.sum need tidying (read-only), then verify
  test-functional   Functional tests: the hurl suite against a fresh bin/<NAME> (needs hurl)
  test-integration  Integration tests (TestIntegration_*, build tag integration; may need Docker)
  mocks             Regenerate mocks/ with mockery (.mockery.yaml)
  tidy              go mod tidy + verify
  vendor            go mod tidy + vendor + verify (vendor/ is git-ignored)
  docker            Build the Docker image
  rename            Adapt the template: make rename MODULE=<module path> NAME=<name>
  clean             Remove build artefacts
```

Before committing, run `make task`: it regenerates code and mocks, tidies
`go.mod`, applies `go fix` and the linters' fixes, formats, then runs
`make ci`. `make ci` only checks (it never edits a file) and runs the checks
of GitHub CI's lint and test jobs. CI also runs govulncheck, the stale-mock
check, the hurl suite (`make test-functional`) and a smoke test of the Docker
image, which `make ci` leaves out.

Tools: Go 1.27 or later, [golangci-lint](https://golangci-lint.run/docs/welcome/install/)
v2.15.0 (the version CI pins), [hurl](https://hurl.dev/), Docker, and four
Go tools: [mockery](https://vektra.github.io/mockery/),
[gotestsum](https://github.com/gotestyourself/gotestsum),
[benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) and
[vacuum](https://quobix.com/vacuum/) (the OpenAPI linter).
`make install-tools` installs those four at the versions CI uses
(`go install`, into `$(go env GOPATH)/bin`); golangci-lint has its own
install page. Tools are installed binaries, not `go tool`s pinned in
`go.mod`: they keep working offline and after `go clean -cache -modcache`,
and they stay out of the service's dependencies. Each target checks the
tools it runs first: a missing tool stops it with the install command, a
different version only warns; `make check-tools` checks them all. Versions
are pinned in the Makefile (`*_VERSION`) and bumped by hand there:
Dependabot does not see them. Tests run through gotestsum:
`make test GOTESTSUM_FORMAT=testname` lists every test.

### Adapting the template

The project's name and module path are written in a few places only;
everything else derives from them:

| Where | What |
|-------|------|
| `Makefile` | `NAME ?= foo`: binaries (`bin/$(NAME)`), image tag, image title |
| `go.mod` | the module path: `MODULE`, the image source label (`SOURCE`), the coverage filter, gci's import grouping (`localmodule`) |
| Go import paths, `.mockery.yaml` | the module path |
| `cmd/server/main.go` | `programName`: usage and `-version` output |
| `internal/config/config.go` | `envPrefix = "FOO_"`: every environment variable name |
| `Dockerfile` | `ARG NAME` and `ARG SOURCE` defaults, for a plain `docker build` (`make docker` passes both) |
| `api/openapi.yaml` | `info.title` and the contact URL |
| `.golangci.yml`, `.golangci.lite.yml` | gofumpt's `module-path`: empty, set only for a module without a dot in its first element (`widget`), whose imports gofumpt would otherwise take for the standard library |
| `README.md`, `AGENTS.md` | prose, badges and examples |

The GitHub workflows read the repository from `${{ github.repository }}`
and the name from `make print-NAME`; the image's entrypoint is a fixed
`/server`. `make build NAME=widget` builds `bin/widget` without editing
anything.

`make rename MODULE=<module path> NAME=<name>` rewrites all of them
(`scripts/rename.sh`): the module path in Go files, `.mockery.yaml`, `go.mod`
and the docs, the name in the Makefile, `programName`, the Dockerfile
defaults, the OpenAPI spec's title and contact URL, and `envPrefix` (the
name upper-cased, `-` → `_`). It then
regenerates the mocks under the new path (mockery must be installed:
`make install-tools` first) and tidies `go.mod`. Running it twice changes
nothing. It leaves the CHANGELOG (history) and the prose alone, warns about
any file that still names the old module, and reminds you of the
per-repository settings it can't change (secrets, branch protection,
Codecov). README badges follow only when the new module is on `github.com`;
a single-element module (`widget`) works too, with a placeholder image
source label.

- **A new project:** copy the template with
  [gonew](https://pkg.go.dev/golang.org/x/tools/cmd/gonew), which rewrites
  the module path and imports, then set the name:

  ```sh
  go run golang.org/x/tools/cmd/gonew@latest github.com/peczenyj/foo example.com/acme/widget
  cd widget && make install-tools && make rename MODULE=example.com/acme/widget NAME=widget
  ```

- **An existing repository** with its own `go.mod`: copy the template's files
  in (not its `go.mod` and `go.sum`), then
  `make install-tools && make rename MODULE=<the module in your go.mod> NAME=<name>`.
  The template's imports are rewritten to your module, `go.mod` is kept, and
  `go mod tidy` adds the template's dependencies.

### Lint profiles

Two golangci-lint configurations, picked with `LINT_PROFILE`:

- **full** (`.golangci.yml`, the default): 64 linters, strict on style
  (`wsl_v5`, `nlreturn`, `lll`, `mnd`, error wrapping with `err113` and
  `wrapcheck`, parallel tests…). This repository's standard, and what CI
  runs. Right for a long-lived service.
- **lite** (`.golangci.lite.yml`): the standard set plus the linters that
  find bugs (HTTP bodies, contexts, error comparison, `gosec`, `sloglint`,
  `testifylint`…), and the same formatters. Right for short, time-boxed work.

`make lint LINT_PROFILE=lite` tries lite once; a project that adopts it
changes `LINT_PROFILE ?= full` in the Makefile, and every lint and format
target follows. Lite is a subset of full, so code that passes full always
passes lite: `make lint-profiles` (part of `make ci`) checks the linter sets
and runs lite. To move a lite project to full, run
`make lint-fix LINT_PROFILE=full` (it fixes the blank-line rules and
formatting itself), then fix what remains by hand.

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
   `bash scripts/hurl.sh http://host:port` tests a server that is already running.

All Go tests use [testify](https://github.com/stretchr/testify).

### Benchmarks

`BenchmarkAPI_GetFoo` measures the `GET /foo` hot path: routing, the real
`Fooer` and JSON encoding. Benchmarks are read by people, not judged by CI:

- `make bench` runs every benchmark `BENCH_COUNT` times (6) with `-benchmem`
  and summarises time, bytes and allocations per operation with
  [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat). A single run is noise; the `±` tells you how much to trust
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

### Releases

1. In a pull request to `devel`, rename `## [Unreleased]` in `CHANGELOG.md`
   to the version and date, and open a new, empty `## [Unreleased]` above it.
2. Merge `devel` into `main` with a pull request, using a merge commit (not
   squash), so both branches keep a shared history.
3. Tag the merge commit (`git tag -a vX.Y.Z`) and push the tag; CI tests it.
4. Once that CI run passes, create the GitHub Release from the tag, with its
   CHANGELOG section as notes. Publishing it runs `release.yml`, which pushes
   `ghcr.io/peczenyj/foo:vX.Y.Z` and `latest` (not for a prerelease).

To publish an existing tag again, run the Release workflow by hand (Actions,
Release, Run workflow) with the tag; `latest` moves only if you tick it.

### Changelog

`CHANGELOG.md` lists the newest changes first: `## [Unreleased]` is the
first section, and each pull request adds its bullets at the top of it.
Released sections are never edited. When two open pull requests both add a
bullet, the second one conflicts on GitHub: keep both bullets, newest first.
