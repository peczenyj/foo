// Package config reads the server settings from command-line flags, with
// FOO_* environment variables as defaults: flag > env var > built-in default.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"time"
)

// Environment variables read by Parse.
const (
	EnvHost            = "FOO_HOST"
	EnvPort            = "FOO_PORT"
	EnvLogLevel        = "FOO_LOG_LEVEL"
	EnvLogFormat       = "FOO_LOG_FORMAT"
	EnvShutdownTimeout = "FOO_SHUTDOWN_TIMEOUT"
)

// Log formats accepted by Parse.
const (
	LogFormatText = "text"
	LogFormatJSON = "json"
)

const maxPort = 65535

// ErrInvalid is returned, wrapped, for an invalid flag or environment value.
var ErrInvalid = errors.New("invalid configuration")

// Config holds the server settings.
type Config struct {
	Host            string        // listen host; empty means all interfaces
	Port            int           // listen port; 0 means any free port
	LogLevel        slog.Level    // minimum level logged; debug enables debug logs
	LogFormat       string        // LogFormatText or LogFormatJSON
	ShutdownTimeout time.Duration // grace period for in-flight requests
	ShowVersion     bool          // print the version and exit
}

// Default returns the built-in settings.
func Default() Config {
	return Config{
		Host:            "",
		Port:            8080,
		LogLevel:        slog.LevelInfo,
		LogFormat:       LogFormatText,
		ShutdownTimeout: 10 * time.Second,
	}
}

// Addr returns the listen address, host:port, with an IPv6 host bracketed.
func (c Config) Addr() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// Parse reads args (without the program name), using getenv for defaults.
// Errors and usage are written to output. It returns flag.ErrHelp for -h,
// and an error wrapping ErrInvalid for an invalid value. With -version, the
// flag values are not validated.
//
// Every invalid environment variable is reported, even when a flag replaces
// it, so a broken deployment setting never hides behind a flag.
func Parse(name string, args []string, getenv func(string) string, output io.Writer) (Config, error) {
	cfg := Default()

	envErr := cfg.readEnv(getenv)

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(output)

	fs.StringVar(&cfg.Host, "host", cfg.Host, "listen host, empty for all interfaces (env "+EnvHost+")")
	fs.IntVar(&cfg.Port, "port", cfg.Port, "listen port, 0 for any free port (env "+EnvPort+")")
	fs.TextVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "minimum log level: debug, info, warn or error (env "+EnvLogLevel+")")
	fs.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "log format: text or json (env "+EnvLogFormat+")")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", cfg.ShutdownTimeout, "grace period for in-flight requests on shutdown (env "+EnvShutdownTimeout+")")
	fs.BoolVar(&cfg.ShowVersion, "version", false, "print the version and exit")

	report := func(err error) error {
		if err != nil {
			fmt.Fprintf(output, "%s: %v\n", name, err)
		}

		return err
	}

	var err error

	switch parseErr := fs.Parse(args); {
	case errors.Is(parseErr, flag.ErrHelp):
		return Config{}, parseErr
	case parseErr != nil:
		// flag has already reported parseErr and the usage.
		return Config{}, errors.Join(fmt.Errorf("%w: %w", ErrInvalid, parseErr), report(envErr))
	case cfg.ShowVersion:
		return cfg, nil
	case fs.NArg() > 0:
		err = fmt.Errorf("%w: unexpected arguments %q", ErrInvalid, fs.Args())
	default:
		err = cfg.validate()
	}

	if err := report(errors.Join(envErr, err)); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// readEnv overrides the defaults with the environment variables that are set.
// It checks each value as validate does, and reports every invalid one; an
// invalid value leaves the built-in default in place.
func (c *Config) readEnv(getenv func(string) string) error {
	var errs []error

	parse := func(name string, set func(string) error) {
		if v := getenv(name); v != "" {
			if err := set(v); err != nil {
				errs = append(errs, fmt.Errorf("%w: %s=%q: %w", ErrInvalid, name, v, err))
			}
		}
	}

	parse(EnvHost, setIfValid(&c.Host, asString))
	parse(EnvPort, setIfValid(&c.Port, checked(strconv.Atoi, checkPort)))
	parse(EnvLogLevel, setIfValid(&c.LogLevel, parseLevel))
	parse(EnvLogFormat, setIfValid(&c.LogFormat, checked(asString, checkLogFormat)))
	parse(EnvShutdownTimeout, setIfValid(&c.ShutdownTimeout, checked(time.ParseDuration, checkShutdownTimeout)))

	return errors.Join(errs...)
}

// setIfValid returns a setter that stores the parsed value in dst only on success,
// so an invalid value leaves the built-in default in place.
func setIfValid[T any](dst *T, parse func(string) (T, error)) func(string) error {
	return func(v string) error {
		x, err := parse(v)
		if err == nil {
			*dst = x
		}

		return err
	}
}

// checked returns parse followed by check, for a value that must parse and
// then pass check.
func checked[T any](parse func(string) (T, error), check func(T) error) func(string) (T, error) {
	return func(v string) (T, error) {
		x, err := parse(v)
		if err == nil {
			err = check(x)
		}

		return x, err
	}
}

func asString(v string) (string, error) { return v, nil }

// parseLevel parses a log level name, such as "debug" or "warn".
func parseLevel(v string) (level slog.Level, err error) {
	err = level.UnmarshalText([]byte(v))

	return level, err
}

func checkPort(port int) error {
	if port < 0 || port > maxPort {
		return fmt.Errorf("must be between 0 and %d", maxPort)
	}

	return nil
}

func checkLogFormat(format string) error {
	if format != LogFormatText && format != LogFormatJSON {
		return fmt.Errorf("must be %q or %q", LogFormatText, LogFormatJSON)
	}

	return nil
}

func checkShutdownTimeout(timeout time.Duration) error {
	if timeout <= 0 {
		return errors.New("must be positive")
	}

	return nil
}

// validate checks the values that parsing alone cannot. It reports every
// invalid value, not just the first one.
func (c *Config) validate() error {
	var errs []error

	if err := checkPort(c.Port); err != nil {
		errs = append(errs, fmt.Errorf("%w: port %d: %w", ErrInvalid, c.Port, err))
	}

	if err := checkLogFormat(c.LogFormat); err != nil {
		errs = append(errs, fmt.Errorf("%w: log format %q: %w", ErrInvalid, c.LogFormat, err))
	}

	if err := checkShutdownTimeout(c.ShutdownTimeout); err != nil {
		errs = append(errs, fmt.Errorf("%w: shutdown timeout %v: %w", ErrInvalid, c.ShutdownTimeout, err))
	}

	return errors.Join(errs...)
}

// NewLogger returns a logger writing to w in the configured format and level.
func (c Config) NewLogger(w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.LogLevel}

	if c.LogFormat == LogFormatJSON {
		return slog.New(slog.NewJSONHandler(w, opts))
	}

	return slog.New(slog.NewTextHandler(w, opts))
}
