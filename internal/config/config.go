// Package config reads the server settings from command-line flags, with
// environment variables (envPrefix + setting) as defaults: flag > env var >
// built-in default.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"
)

// envPrefix starts every environment variable name: the project name, upper
// case, with "-" replaced by "_" (make rename sets it).
const envPrefix = "FOO_"

// Environment variables read by Parse.
const (
	EnvHost            = envPrefix + "HOST"
	EnvPort            = envPrefix + "PORT"
	EnvLogLevel        = envPrefix + "LOG_LEVEL"
	EnvLogFormat       = envPrefix + "LOG_FORMAT"
	EnvShutdownTimeout = envPrefix + "SHUTDOWN_TIMEOUT"
)

// Log formats accepted by Parse.
const (
	LogFormatText = "text"
	LogFormatJSON = "json"
)

const (
	defaultPort            = 8080
	defaultShutdownTimeout = 10 * time.Second
	maxPort                = 65535
)

// ErrInvalid is returned, wrapped, for an invalid flag or environment value.
var ErrInvalid = errors.New("invalid configuration")

// Why a value is invalid, wrapped after ErrInvalid in the errors Parse returns.
var (
	errHost            = errors.New("must be a host name or an IP address, without a port (brackets only around IPv6)")
	errPortRange       = errors.New("must be between 0 and 65535")
	errLogFormat       = errors.New(`must be "text" or "json"`)
	errLogLevel        = errors.New("must be debug, info, warn or error")
	errShutdownTimeout = errors.New("must be positive")
)

// Config holds the server settings.
type Config struct {
	Host            string        // listen host, never bracketed; empty means all interfaces
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
		Port:            defaultPort,
		LogLevel:        slog.LevelInfo,
		LogFormat:       LogFormatText,
		ShutdownTimeout: defaultShutdownTimeout,
	}
}

// Addr returns the listen address, host:port, with an IPv6 host bracketed.
func (c Config) Addr() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// Parse reads args (without the program name), using getenv for defaults.
// Errors and usage are written to output. It returns flag.ErrHelp for -h,
// and an error wrapping ErrInvalid for an invalid value, with -version too.
//
// Every invalid environment variable is reported, even when a flag replaces
// it or -version is set, so a broken deployment setting never hides. The one
// exception: -version does not require the required secrets, so that
// "<image> -version" works without the deployment's secrets.
func Parse(name string, args []string, getenv func(string) string, output io.Writer) (Config, error) {
	cfg := Default()

	envErrs := readEnv(&cfg, getenv)
	envErr := errors.Join(envErrs...)

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(output)

	fs.StringVar(&cfg.Host, "host", cfg.Host, "listen host, empty for all interfaces (env "+EnvHost+")")
	fs.IntVar(&cfg.Port, "port", cfg.Port, "listen port, 0 for any free port (env "+EnvPort+")")
	fs.Var(levelFlag{&cfg.LogLevel}, "log-level", "minimum log level: debug, info, warn or error (env "+EnvLogLevel+")")
	fs.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "log format: text or json (env "+EnvLogFormat+")")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", cfg.ShutdownTimeout,
		"grace period for in-flight requests on shutdown (env "+EnvShutdownTimeout+")")
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
		return Config{}, flag.ErrHelp
	case parseErr != nil:
		// flag has already reported parseErr and the usage.
		return Config{}, errors.Join(fmt.Errorf("%w: %w", ErrInvalid, parseErr), report(envErr))
	case fs.NArg() > 0:
		// Still validate the flags: every invalid value is reported.
		err = errors.Join(fmt.Errorf("%w: unexpected arguments %q", ErrInvalid, fs.Args()), cfg.validate())
	default:
		err = cfg.validate()
	}

	if cfg.ShowVersion {
		envErr = errors.Join(ignoreMissingSecrets(envErrs)...)
	}

	// An env var host was stripped by readEnv; a -host flag is stripped here.
	if host, hostErr := parseHost(cfg.Host); hostErr == nil {
		cfg.Host = host
	}

	if err := report(errors.Join(envErr, err)); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// readEnv overrides the defaults in c with the environment variables that are
// set. It checks each value as validate does, and returns an error for every
// invalid one (nil entries are fine); an invalid value leaves the built-in
// default in place.
func readEnv(c *Config, getenv func(string) string) []error {
	var errs []error

	parse := func(name string, set func(string) error) {
		if v := getenv(name); v != "" {
			if err := set(v); err != nil {
				errs = append(errs, fmt.Errorf("%w: %s=%q: %w", ErrInvalid, name, v, err))
			}
		}
	}

	parse(EnvHost, setIfValid(&c.Host, parseHost))
	parse(EnvPort, setIfValid(&c.Port, checked(strconv.Atoi, checkPort)))
	parse(EnvLogLevel, setIfValid(&c.LogLevel, parseLevel))
	parse(EnvLogFormat, setIfValid(&c.LogFormat, checked(asString, checkLogFormat)))
	parse(EnvShutdownTimeout, setIfValid(&c.ShutdownTimeout, checked(time.ParseDuration, checkShutdownTimeout)))

	return errs
}

// ignoreMissingSecrets returns errs without the errors of required secrets
// that are not set (errSecretRequired), for -version. A secret that is set
// but invalid (both variables, an unreadable or too-large file) stays.
func ignoreMissingSecrets(errs []error) []error {
	return slices.DeleteFunc(slices.Clone(errs), func(err error) bool { return errors.Is(err, errSecretRequired) })
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

// parseLevel parses a log level name: debug, info, warn or error, in any
// case. slog itself also accepts offsets ("info+2") and other spellings,
// which the help text doesn't list: they are rejected.
func parseLevel(v string) (slog.Level, error) {
	switch strings.ToLower(v) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}

	return 0, errLogLevel
}

// levelFlag is the -log-level flag.Value: parseLevel, so the flag accepts
// exactly what the environment variable does.
type levelFlag struct{ level *slog.Level }

func (f levelFlag) String() string {
	if f.level == nil { // the zero value, which flag uses to detect the default
		return ""
	}

	return strings.ToLower(f.level.String())
}

func (f levelFlag) Set(v string) error {
	level, err := parseLevel(v)
	if err == nil {
		*f.level = level
	}

	return err
}

// parseHost returns host without the brackets of an IPv6 address ("[::1]",
// the form addresses are often copied in), so that Addr brackets it once.
// It rejects a host that Addr would turn into an address nothing can listen
// on, such as "localhost:8080" (a host with a colon must be an IPv6 address),
// and brackets around anything but an IPv6 address.
func parseHost(host string) (string, error) {
	if inner, ok := strings.CutPrefix(host, "["); ok {
		inner, ok = strings.CutSuffix(inner, "]")
		if addr, err := netip.ParseAddr(inner); !ok || err != nil || !addr.Is6() {
			return "", errHost
		}

		return inner, nil
	}

	if strings.Contains(host, ":") {
		if _, err := netip.ParseAddr(host); err != nil {
			return "", errHost
		}
	}

	return host, nil
}

func checkPort(port int) error {
	if port < 0 || port > maxPort {
		return errPortRange
	}

	return nil
}

func checkLogFormat(format string) error {
	if format != LogFormatText && format != LogFormatJSON {
		return errLogFormat
	}

	return nil
}

func checkShutdownTimeout(timeout time.Duration) error {
	if timeout <= 0 {
		return errShutdownTimeout
	}

	return nil
}

// NewLogger returns a logger writing to w in the configured format and level.
func (c Config) NewLogger(w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.LogLevel}

	if c.LogFormat == LogFormatJSON {
		return slog.New(slog.NewJSONHandler(w, opts))
	}

	return slog.New(slog.NewTextHandler(w, opts))
}

// validate checks the values that parsing alone cannot. It reports every
// invalid value, not just the first one.
func (c Config) validate() error {
	var errs []error

	if _, err := parseHost(c.Host); err != nil {
		errs = append(errs, fmt.Errorf("%w: host %q: %w", ErrInvalid, c.Host, err))
	}

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
