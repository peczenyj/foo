# Changelog

All notable changes to this project are documented here, and this project
adheres to [Semantic Versioning](https://semver.org/).

Newest first: `## [Unreleased]` is the first section, and each pull request
adds its bullets at the top of it. To release, rename `[Unreleased]` to the
version and date, then open a new, empty `## [Unreleased]` above it.
Released sections are never edited.

## [Unreleased]

- Security response headers (#53) on every public response, the router's
  404 and 405 included: `X-Content-Type-Options: nosniff`,
  `Cache-Control: no-store`, a deny-all `Content-Security-Policy`,
  `X-Frame-Options: DENY` and `Referrer-Policy: no-referrer`.
- A panicking handler is answered with a `500` problem (#53), logged with
  its stack, instead of a dropped connection.
- Middlewares (#53): what applies to every request (`Headers`, `Metrics`,
  `Recover`) moved out of the API into `internal/middleware`, stacked by
  `newHandler` in `cmd/server`; the API only routes. Problem responses are
  written by `internal/problem`.
- A Kubernetes service link in `FOO_PORT` (a Service named `foo`) is ignored
  with a warning instead of stopping the server.
- Admin server (#4), off by default: `-admin-port`/`FOO_ADMIN_PORT` (`0`: any
  free port) and `-admin-host`/`FOO_ADMIN_HOST` (`127.0.0.1` by default)
  start a second HTTP server with `GET /debug/vars` (expvar) and
  `/debug/pprof/*`, on its own mux and port. Both addresses are bound before
  either server starts; it shuts down with the public server, and if either
  fails, both stop. A shutdown ends a running CPU profile or trace early
  instead of waiting out its `seconds`.
- Request metrics in `/debug/vars`: `http_requests_total` and
  `http_request_duration_seconds_total`, keyed by route pattern (or
  `unmatched`) and status, recorded through the `metrics.Recorder` interface
  (`internal/metrics`, expvar); `middleware.Metrics` records with each
  recorder it is given, so another backend can be added later. Streaming,
  connection hijacking (recorded as `101`) and `sendfile` keep working
  behind it.
- The "closing server" log line names the address being closed.

## [0.5.0] - 2026-11-22

- `-version` no longer hides invalid settings: an invalid flag or environment
  variable is reported, with exit status 2, as without it. It still doesn't
  require the required secrets, so `<image> -version` works without them.
- `-host` and `FOO_HOST` accept a bracketed IPv6 address (`[::1]`) and strip
  the brackets; brackets around anything else are a usage error (exit 2)
  instead of a listen failure (exit 1).
- `-log-level` and `FOO_LOG_LEVEL` accept only `debug`, `info`, `warn` and
  `error` (in any case): slog's other forms (`info+2`, numbers) are rejected,
  as the help text always said. The help shows the default as `info`.
- Tests: `HEAD /foo` (component test, also in the lite kit, and hurl); the
  address-in-use test checks the error on every OS, not Linux's message.
- Docs: a plain `docker build` leaves the `created` label empty (README,
  Dockerfile); `make docker` sets it.
- Lite kit: `make export-lite DEST=<dir> MODULE=<path> NAME=<name> [FORCE=1]`
  copies a smaller, renamed copy of the template (no mocks, hurl,
  benchmarks, OpenAPI, Codecov or release automation; golangci-lint as the
  only tool; testify kept, a hand-written fake instead of mocks) into
  another repository. It never overwrites a file without `FORCE=1`. A
  `lite-kit` CI job runs the export's `make ci`.
- `make rename` no longer needs `.mockery.yaml`, and warns about GitHub badges
  only in the Markdown files that still have them.
- `.gitlab-ci.yml`: the GitHub CI checks for projects hosted on GitLab, through
  the same make targets (lint, openapi, test, mocks, vulncheck, functional,
  and a Docker-in-Docker image build smoke-tested with the hurl suite), with
  JUnit test results and Cobertura coverage in merge requests. New
  `make cover-cobertura` and installed tool gocover-cobertura.
- CI caches the installed tools (vacuum, oasdiff, gotestsum, mockery),
  keyed on the versions pinned in the Makefile: the lint job no longer
  compiles vacuum and oasdiff on every run (about 280 s).

## [0.4.0] - 2026-11-22

- `make openapi-breaking` (oasdiff, a new installed tool), in `make ci` and
  CI, fails on OpenAPI changes that break clients of the last final release;
  a deliberate break is listed in `api/oasdiff-ignore.txt` in the same pull
  request.
- The OpenAPI spec is attached to each GitHub Release
  (`releases/download/vX.Y.Z/openapi.yaml`, a stable URL for client
  generation), and CI checks on tag pushes that its `info.version` matches
  the tag (`scripts/openapi-version.sh`); the release pull request bumps it.
- OpenAPI: `api/openapi.yaml` (3.0.3) describes `GET /foo` (200, and 400 and
  500 as problem details) and `GET /healthz`, so clients can be generated
  from it. `make openapi-lint` (vacuum, a new installed tool) runs in
  `make ci`; a contract test checks that the registered routes and the
  spec's operations match both ways, that every test response matches the
  spec, and that every documented response is produced.

## [0.3.0] - 2026-11-22

- Secrets from files: every secret can also be read from the file that
  `<NAME>_FILE` names (the Docker and Kubernetes secrets convention), once at
  startup and up to 64 KiB, keeping the value out of the process
  environment. Both variables set, or a file that can't be read or is too
  large, is a usage error (exit 2).
- Secrets: `config.Secret` prints, logs and encodes as `REDACTED` (every
  `fmt` verb, `slog`, JSON), or `(unset)` without a value, and returns its
  value only through `Reveal()`; secrets are environment-only settings
  (`readSecret`, which drops a trailing newline), and a missing required
  one is a usage error (exit 2). foo declares none; the README
  shows the three lines to add one.
- Errors from `GET /foo` are RFC 9457 problem details
  (`application/problem+json`) instead of plain text: `500` with a generic
  `detail` (the error is logged), and `400` with only its `Detail` when a
  `Fooer` returns a `*foo.InputError`, however it is wrapped. `problemFor`
  is the single error-to-status mapping.
  The router's `404` and `405` keep their plain-text body.

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
