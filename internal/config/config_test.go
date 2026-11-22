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

	cfg, err := config.Parse("server", args, env(vars), &output)

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
	assert.False(t, cfg.AdminEnabled(), "the admin server is off unless a port is set")
	assert.Equal(t, -1, cfg.AdminPort)
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
		{
			label: "should accept an IPv6 host with a zone",
			env:   map[string]string{config.EnvHost: "fe80::1%eth0"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, "[fe80::1%eth0]:8080", cfg.Addr()) },
		},
		{
			label: "should enable the admin server with -admin-port",
			args:  []string{"-admin-port=9090"},
			check: func(t *testing.T, cfg config.Config) {
				assert.True(t, cfg.AdminEnabled())
				assert.Equal(t, ":9090", cfg.AdminAddr())
			},
		},
		{
			label: "should enable the admin server on any free port with 0",
			env:   map[string]string{config.EnvAdminPort: "0"},
			check: func(t *testing.T, cfg config.Config) {
				assert.True(t, cfg.AdminEnabled())
				assert.Equal(t, ":0", cfg.AdminAddr())
			},
		},
		{
			label: "should let -admin-port=-1 turn off an admin port env var",
			args:  []string{"-admin-port=-1"},
			env:   map[string]string{config.EnvAdminPort: "9090"},
			check: func(t *testing.T, cfg config.Config) { assert.False(t, cfg.AdminEnabled()) },
		},
		{
			label: "should read the admin host, brackets stripped",
			args:  []string{"-admin-port=9090"},
			env:   map[string]string{config.EnvAdminHost: "[::1]"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, "[::1]:9090", cfg.AdminAddr()) },
		},
		{
			label: "should strip the brackets of an admin host flag",
			args:  []string{"-admin-host=[::1]", "-admin-port=9090"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, "::1", cfg.AdminHost) },
		},
		{
			label: "should accept port 0 for both servers",
			args:  []string{"-port=0", "-admin-port=0"},
			check: func(t *testing.T, cfg config.Config) { assert.True(t, cfg.AdminEnabled()) },
		},
		{
			label: "should strip the brackets of an IPv6 host flag",
			args:  []string{"-host=[::1]"},
			check: func(t *testing.T, cfg config.Config) {
				assert.Equal(t, "::1", cfg.Host)
				assert.Equal(t, "[::1]:8080", cfg.Addr())
			},
		},
		{
			label: "should strip the brackets of an IPv6 host env var",
			env:   map[string]string{config.EnvHost: "[fe80::1%eth0]"},
			check: func(t *testing.T, cfg config.Config) {
				assert.Equal(t, "fe80::1%eth0", cfg.Host)
				assert.Equal(t, "[fe80::1%eth0]:8080", cfg.Addr())
			},
		},
		{
			label: "should accept a log level in any case",
			args:  []string{"-log-level=WARN"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, slog.LevelWarn, cfg.LogLevel) },
		},
		{
			label: "should let -log-level=info replace a debug env var",
			args:  []string{"-log-level=Info"},
			env:   map[string]string{config.EnvLogLevel: "debug"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, slog.LevelInfo, cfg.LogLevel) },
		},
		{
			label: "should accept a log level env var in any case",
			env:   map[string]string{config.EnvLogLevel: "Error"},
			check: func(t *testing.T, cfg config.Config) { assert.Equal(t, slog.LevelError, cfg.LogLevel) },
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
			label:  "should reject a non-numeric port env var",
			env:    map[string]string{config.EnvPort: "abc"},
			output: config.EnvPort,
		},
		{
			label:  "should reject a port env var above 65535",
			env:    map[string]string{config.EnvPort: "70000"},
			output: config.EnvPort,
		},
		{
			label:  "should reject a negative port env var",
			env:    map[string]string{config.EnvPort: "-1"},
			output: config.EnvPort,
		},
		{
			label:  "should report an invalid port env var even when -port replaces it",
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
		{label: "should reject extra arguments", args: []string{"extra"}, output: "unexpected arguments"},
		{
			label:  "should still validate the flags next to extra arguments",
			args:   []string{"-port=70000", "extra"},
			output: "port 70000",
		},
		{label: "should reject a host with a port", args: []string{"-host=localhost:80"}, output: `host "localhost:80"`},
		{label: "should reject a bracketed host name", args: []string{"-host=[localhost]"}, output: `host "[localhost]"`},
		{label: "should reject a bracketed IPv4 address", args: []string{"-host=[127.0.0.1]"}, output: `host "[127.0.0.1]"`},
		{
			label:  "should reject a bracketed host env var that is not IPv6",
			env:    map[string]string{config.EnvHost: "[localhost]"},
			output: config.EnvHost,
		},
		{
			label:  "should reject a log level with an offset",
			args:   []string{"-log-level=INFO+2"},
			output: "must be debug, info, warn or error",
		},
		{
			label:  "should reject a log level env var with an offset",
			env:    map[string]string{config.EnvLogLevel: "debug-4"},
			output: config.EnvLogLevel,
		},
		{
			label:  "should reject a numeric log level",
			args:   []string{"-log-level=0"},
			output: "must be debug, info, warn or error",
		},
		{label: "should reject an admin port below -1", args: []string{"-admin-port=-2"}, output: "admin port -2"},
		{label: "should reject an admin port above 65535", args: []string{"-admin-port=65536"}, output: "admin port 65536"},
		{
			label:  "should reject an invalid admin port env var",
			env:    map[string]string{config.EnvAdminPort: "abc"},
			output: config.EnvAdminPort,
		},
		{
			label:  "should reject the public port as the admin port",
			args:   []string{"-port=9090", "-admin-port=9090"},
			output: "admin port 9090: must differ from the port",
		},
		{
			label:  "should reject an admin host with a port",
			args:   []string{"-admin-host=localhost:1", "-admin-port=9090"},
			output: `admin host "localhost:1"`,
		},
		{
			label:  "should reject an admin host env var with a port",
			env:    map[string]string{config.EnvAdminHost: "localhost:1"},
			output: config.EnvAdminHost,
		},
		{
			label:  "should report an invalid env var with -version",
			args:   []string{"-version"},
			env:    map[string]string{config.EnvPort: "abc"},
			output: config.EnvPort,
		},
		{
			label:  "should report an invalid flag with -version",
			args:   []string{"-version", "-port=70000"},
			output: "port 70000",
		},
		{
			label:  "should reject a host env var with a port",
			env:    map[string]string{config.EnvHost: "localhost:80"},
			output: config.EnvHost,
		},
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

func TestParse_version(t *testing.T) {
	t.Parallel()

	cfg, _, err := parse(t, []string{"-version"}, nil)
	require.NoError(t, err)

	assert.True(t, cfg.ShowVersion)
}

func TestParse_help_lists_the_log_levels(t *testing.T) {
	t.Parallel()

	_, output, err := parse(t, []string{"-h"}, nil)
	require.ErrorIs(t, err, flag.ErrHelp)

	assert.Contains(t, output, "minimum log level: debug, info, warn or error")
	assert.Contains(t, output, `(default info)`)
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
