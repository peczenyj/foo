// Command server runs the HTTP service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/peczenyj/foo/internal/admin"
	"github.com/peczenyj/foo/internal/api"
	"github.com/peczenyj/foo/internal/config"
	"github.com/peczenyj/foo/internal/foo"
	"github.com/peczenyj/foo/internal/metrics"
	"github.com/peczenyj/foo/internal/middleware"
)

const (
	programName = "foo"

	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 10 * time.Second
	defaultWriteTimeout      = 10 * time.Second
	defaultIdleTimeout       = 120 * time.Second
	defaultMaxHeaderBytes    = 16 << 10 // 16 KiB
)

// Set at build time with -ldflags "-X main.version=... -X main.revision=...".
//
//nolint:gochecknoglobals // -ldflags -X can only set package-level variables.
var (
	version  = "dev"
	revision = "unknown"
)

// Exit codes returned by run.
const (
	exitOK    = 0
	exitError = 1 // the server failed
	exitUsage = 2 // invalid flags or environment, as for flag errors
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// run reads the configuration, then serves until ctx is cancelled or SIGINT
// or SIGTERM arrives. It returns the process exit code.
func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	cfg, err := config.Parse(programName, args, getenv, stderr)

	switch {
	case errors.Is(err, flag.ErrHelp):
		return exitOK
	case err != nil:
		return exitUsage // already reported by config.Parse
	case cfg.ShowVersion:
		fmt.Fprintf(stdout, "%s %s (revision %s)\n", programName, version, revision)

		return exitOK
	}

	logger := cfg.NewLogger(stderr)
	slog.SetDefault(logger)

	logger.Info("application start",
		slog.String("version", version),
		slog.String("revision", revision),
		slog.String("log_level", cfg.LogLevel.String()),
		slog.String("log_format", cfg.LogFormat),
	)

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Once the first signal starts the graceful shutdown, restore the default
	// signal behaviour: a second SIGINT or SIGTERM kills a shutdown that hangs.
	context.AfterFunc(ctx, cancel)

	if err := serveAll(ctx, cfg, newHandler(logger), logger); err != nil {
		logger.Error("server finished with error", slog.Any("error", err))

		return exitError
	}

	return exitOK
}

// newHandler returns the public server's handler: the API, which only
// routes and answers, inside the middlewares that apply to every request,
// outermost first.
func newHandler(logger *slog.Logger) http.Handler {
	return middleware.Chain(api.New(foo.New(), logger),
		middleware.Metrics(logger, metrics.Expvar()),
		middleware.Recover(logger),
	)
}

var (
	errServerStopped = errors.New("server stopped")
	errOtherFailed   = errors.New("the other server failed")
)

// serveAll runs the public server and, when cfg enables it, the admin server
// (package admin), with runAll. Both addresses are bound before either server
// starts: if one cannot listen, nothing is served or logged as listening.
func serveAll(ctx context.Context, cfg config.Config, handler http.Handler, logger *slog.Logger) error {
	listener, err := listen(ctx, cfg.Addr())
	if err != nil {
		return err
	}

	servers := []namedServer{{run: func(ctx context.Context) error {
		return serve(ctx, "listening", listener, newHTTPServer(handler, logger), cfg.ShutdownTimeout, logger)
	}}}

	if cfg.AdminEnabled {
		adminListener, err := listen(ctx, cfg.AdminAddr())
		if err != nil {
			_ = listener.Close()

			return fmt.Errorf("admin server: %w", err)
		}

		servers = append(servers, namedServer{name: "admin server", run: func(ctx context.Context) error {
			server := newAdminServer(ctx, admin.New(), logger)

			return serve(ctx, "admin listening", adminListener, server, cfg.ShutdownTimeout, logger)
		}})
	}

	return runAll(ctx, servers...)
}

// namedServer is a server runAll runs: run serves until its ctx is cancelled.
type namedServer struct {
	name string // prefixes its errors; empty for the public server
	run  func(ctx context.Context) error
}

// runAll runs servers until ctx is cancelled or one of them fails: then it
// cancels the others too (cause errOtherFailed), so the service never runs
// half-up. It returns the servers' errors, each prefixed with its server's
// name, joined.
func runAll(ctx context.Context, servers ...namedServer) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(errServerStopped)

	errs := make(chan error, len(servers))

	for _, server := range servers {
		go func() {
			err := server.run(ctx)
			if err != nil {
				cancel(errOtherFailed)

				if server.name != "" {
					err = fmt.Errorf("%s: %w", server.name, err)
				}
			}

			errs <- err
		}()
	}

	all := make([]error, 0, len(servers))

	for range servers {
		all = append(all, <-errs)
	}

	return errors.Join(all...)
}

// newAdminServer returns the http.Server of the admin handler: the public
// server's timeouts (pprof extends its own write deadline by ?seconds= for
// CPU profiles and traces), and request contexts derived from ctx.
//
// Unlike the public server, its in-flight requests are cancelled when ctx is:
// a CPU profile or trace runs until its request context ends, so a shutdown
// would otherwise wait out its ?seconds= (and time out). Cancelled, it ends
// early and is still written.
func newAdminServer(ctx context.Context, handler http.Handler, logger *slog.Logger) *http.Server {
	server := newHTTPServer(handler, logger)
	server.BaseContext = func(net.Listener) context.Context { return ctx }

	return server
}

// listen binds addr.
func listen(ctx context.Context, addr string) (net.Listener, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	return listener, nil
}

// serve serves server on listener until ctx is cancelled, then shuts it down
// gracefully within timeout. It logs msg with the bound address.
func serve(
	ctx context.Context,
	msg string,
	listener net.Listener,
	server *http.Server,
	timeout time.Duration,
	logger *slog.Logger,
) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(errServerStopped)

	done := make(chan error, 1)

	bound := listener.Addr().String()

	go prepareServerShutdown(ctx, server, timeout, logger.With(slog.String("addr", bound)), done)

	logger.Info(msg, slog.String("addr", bound))

	// Serve closes the listener, and returns immediately during graceful shutdown.
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("unexpected error from http serve: %w", err)
	}

	return <-done
}

// newHTTPServer returns the http.Server for handler. net/http reports handler
// panics and accept errors through ErrorLog: they go to logger at error level,
// so a log level above info never drops them.
func newHTTPServer(handler http.Handler, logger *slog.Logger) *http.Server {
	return &http.Server{
		Handler:           handler,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
		MaxHeaderBytes:    defaultMaxHeaderBytes,
	}
}

func prepareServerShutdown(
	ctx context.Context,
	server *http.Server,
	timeout time.Duration,
	logger *slog.Logger,
	done chan error,
) {
	defer close(done)

	<-ctx.Done()

	if cause := context.Cause(ctx); !errors.Is(cause, errServerStopped) {
		logger.Info("closing server", slog.Any("cause", cause))
	}

	// ctx is already cancelled: keep its values, not its cancellation.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		done <- fmt.Errorf("http server shutdown error: %w", err)
	}
}
