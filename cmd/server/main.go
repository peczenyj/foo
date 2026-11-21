// Command server runs the foo HTTP service.
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

	"github.com/peczenyj/foo/internal/api"
	"github.com/peczenyj/foo/internal/config"
	"github.com/peczenyj/foo/internal/foo"
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

	if err := RunServer(ctx, cfg, api.New(foo.New(), logger), logger); err != nil {
		logger.Error("server finished with error", slog.Any("error", err))

		return exitError
	}

	return exitOK
}

var errServerStopped = errors.New("server stopped")

// RunServer serves handler on cfg.Addr() until ctx is cancelled, then shuts
// down gracefully within cfg.ShutdownTimeout. It logs the bound address,
// which differs from cfg.Addr() when the port is 0.
func RunServer(ctx context.Context, cfg config.Config, handler http.Handler, logger *slog.Logger) error {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.Addr())
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(errServerStopped)

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
		MaxHeaderBytes:    defaultMaxHeaderBytes,
	}

	done := make(chan error, 1)

	go prepareServerShutdown(ctx, server, cfg.ShutdownTimeout, logger, done)

	logger.Info("listening", slog.String("addr", listener.Addr().String()))

	// Serve closes the listener, and returns immediately during graceful shutdown.
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("unexpected error from http serve: %w", err)
	}

	return <-done
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
