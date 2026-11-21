package config_test

import (
	"bytes"
	"context"
	"flag"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/config"
)

// env returns a getenv function reading from vars.
func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func parse(t *testing.T, args []string, vars map[string]string) (config.Config, string, error) {
	t.Helper()

	var output bytes.Buffer

	cfg, err := config.Parse("foo", args, env(vars), &output)

	return cfg, output.String(), err
}

func TestParse_defaults(t *testing.T) {
	t.Parallel()

	cfg, output, err := parse(t, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, config.Default(), cfg)
	assert.Equal(t, ":8080", cfg.Addr())
	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
	assert.Equal(t, config.LogFormatText, cfg.LogFormat)
	assert.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
	assert.Empty(t, output)
}

func TestParse(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		label string
		args  []string
		env   map[string]string
		check func(t *testing.T, cfg config.Config)
	}{
		{
			label: "should read host and port from the environment",
			env:   map[string]string{config.EnvHost: "127.0.0.1", config.EnvPort: "9090"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, "127.0.0.1:9090", cfg.Addr()) },
		},
		{
			label: "should read host and port from flags",
			args:  []string{"-host=localhost", "-port=7070"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, "localhost:7070", cfg.Addr()) },
		},
		{
			label: "should prefer a flag to the environment",
			args:  []string{"-port=7070"},
			env:   map[string]string{config.EnvPort: "9090"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, 7070, cfg.Port) },
		},
		{
			label: "should enable debug from a flag",
			args:  []string{"-log-level=debug"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, slog.LevelDebug, cfg.LogLevel) },
		},
		{
			label: "should enable debug from the environment",
			env:   map[string]string{config.EnvLogLevel: "debug"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, slog.LevelDebug, cfg.LogLevel) },
		},
		{
			label: "should enable JSON logs from a flag",
			args:  []string{"-log-format=json"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, config.LogFormatJSON, cfg.LogFormat) },
		},
		{
			label: "should enable JSON logs from the environment",
			env:   map[string]string{config.EnvLogFormat: "json"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, config.LogFormatJSON, cfg.LogFormat) },
		},
		{
			label: "should read the shutdown timeout from the environment",
			env:   map[string]string{config.EnvShutdownTimeout: "3s"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, 3*time.Second, cfg.ShutdownTimeout) },
		},
		{
			label: "should accept port 0 (any free port)",
			args:  []string{"-port=0"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, 0, cfg.Port) },
		},
		{
			label: "should bracket an IPv6 host",
			args:  []string{"-host=::1"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, "[::1]:8080", cfg.Addr()) },
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			cfg, output, err := parse(t, tc.args, tc.env)
			require.NoError(t, err, output)

			tc.check(t, cfg)
		})
	}
}

func TestParse_errors(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		label  string
		args   []string
		env    map[string]string
		output string // expected substring
	}{
		{
			label:  "should reject a non-numeric FOO_PORT",
			env:    map[string]string{config.EnvPort: "abc"},
			output: config.EnvPort,
		},
		{
			label:  "should reject a FOO_PORT above 65535",
			env:    map[string]string{config.EnvPort: "70000"},
			output: config.EnvPort,
		},
		{
			label:  "should reject a negative FOO_PORT",
			env:    map[string]string{config.EnvPort: "-1"},
			output: config.EnvPort,
		},
		{
			label:  "should report an invalid FOO_PORT even when -port replaces it",
			args:   []string{"-port=7070"},
			env:    map[string]string{config.EnvPort: "abc"},
			output: config.EnvPort,
		},
		{label: "should reject a port above 65535", args: []string{"-port=65536"}, output: "port 65536"},
		{label: "should reject an unknown log format", args: []string{"-log-format=xml"}, output: `log format "xml"`},
		{
			label:  "should reject an unknown log level",
			env:    map[string]string{config.EnvLogLevel: "loud"},
			output: config.EnvLogLevel,
		},
		{label: "should reject a zero shutdown timeout", args: []string{"-shutdown-timeout=0s"}, output: "shutdown timeout"},
		{label: "should reject extra arguments", args: []string{"foo"}, output: "unexpected arguments"},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			_, output, err := parse(t, tc.args, tc.env)
			require.ErrorIs(t, err, config.ErrInvalid)

			assert.Contains(t, output, tc.output)
		})
	}
}

func TestParse_help(t *testing.T) {
	t.Parallel()

	_, output, err := parse(t, []string{"-h"}, nil)
	require.ErrorIs(t, err, flag.ErrHelp)

	assert.Contains(t, output, "-port")
	assert.Contains(t, output, config.EnvPort)
}

// -version skips flag validation: printing the version
// must work whatever else is on the command line.
func TestParse_version_skips_flag_validation(t *testing.T) {
	t.Parallel()

	cfg, _, err := parse(t, []string{"-version", "-port=70000"}, nil)
	require.NoError(t, err)

	assert.True(t, cfg.ShowVersion)
}

func TestNewLogger(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		label     string
		cfg       config.Config
		jsonLines bool
		debug     bool
	}{
		{label: "should write text at info by default", cfg: config.Default()},
		{
			label:     "should write JSON",
			cfg:       config.Config{LogFormat: config.LogFormatJSON, LogLevel: slog.LevelInfo},
			jsonLines: true,
		},
		{
			label: "should log debug in debug mode",
			cfg:   config.Config{LogFormat: config.LogFormatText, LogLevel: slog.LevelDebug},
			debug: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			logger := tc.cfg.NewLogger(&buf)
			logger.Debug("debug line")
			logger.Info("info line")

			out := buf.String()

			assert.Contains(t, out, "info line")
			assert.Equal(t, tc.debug, strings.Contains(out, "debug line"))
			assert.Equal(t, tc.jsonLines, strings.HasPrefix(out, "{"))
			assert.Equal(t, tc.debug, logger.Enabled(context.Background(), slog.LevelDebug))
		})
	}
}
