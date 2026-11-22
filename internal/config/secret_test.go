package config_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peczenyj/foo/internal/config"
)

const (
	secretValue = "hunter2"
	redacted    = "REDACTED"
)

// settings is how a service would hold a secret: next to other settings, in
// an exported field, and (by mistake) in an unexported one.
type settings struct {
	Name   string
	APIKey config.Secret
	token  config.Secret
}

func newSettings() settings {
	return settings{Name: "svc", APIKey: config.NewSecret(secretValue), token: config.NewSecret(secretValue)}
}

func TestSecret_never_prints_its_value(t *testing.T) {
	t.Parallel()

	secret := config.NewSecret(secretValue)
	holder := newSettings()

	values := map[string]any{"secret": secret, "&secret": &secret, "settings": holder, "&settings": &holder}

	for name, v := range values {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
			assert.NotContains(t, fmt.Sprintf(verb, v), secretValue, "%s with %s", name, verb)
		}
	}

	assert.Equal(t, redacted, fmt.Sprint(secret))
	assert.Equal(t, redacted, fmt.Sprintf("%#v", secret))
	assert.Equal(t, redacted, secret.String())
	// fmt calls Format, even for %#v; GoString still serves direct callers.
	assert.Equal(t, redacted, secret.GoString())
	assert.Contains(t, fmt.Sprintf("%+v", holder), "APIKey:"+redacted)
}

func TestSecret_never_logs_its_value(t *testing.T) {
	t.Parallel()

	holder := newSettings()

	for name, newHandler := range map[string]func(*bytes.Buffer) slog.Handler{
		"text": func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
		"json": func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
	} {
		var logs bytes.Buffer

		logger := slog.New(newHandler(&logs))
		logger.Info("config", slog.Any("settings", holder), slog.Any("key", holder.APIKey))
		logger.Info("config", "raw", holder.APIKey, "ptr", &holder.APIKey)

		assert.NotContains(t, logs.String(), secretValue, name)
		assert.Contains(t, logs.String(), redacted, name)
	}
}

func TestSecret_never_encodes_its_value(t *testing.T) {
	t.Parallel()

	holder := newSettings()

	out, err := json.Marshal(holder)
	require.NoError(t, err)
	assert.JSONEq(t, `{"Name":"svc","APIKey":"REDACTED"}`, string(out))

	text, err := holder.APIKey.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, redacted, string(text))
}

func TestSecret_Reveal(t *testing.T) {
	t.Parallel()

	assert.Equal(t, secretValue, config.NewSecret(secretValue).Reveal())
	assert.True(t, config.NewSecret(secretValue).IsSet())

	var zero config.Secret

	assert.Empty(t, zero.Reveal())
	assert.False(t, zero.IsSet())
}

// An unset secret shows "(unset)", so a config dump tells a missing secret
// from a configured one without revealing anything.
func TestSecret_unset_shows_unset(t *testing.T) {
	t.Parallel()

	var holder settings

	assert.Equal(t, "(unset)", holder.APIKey.String())
	assert.Equal(t, "(unset)", holder.APIKey.GoString())
	assert.Equal(t, "(unset)", fmt.Sprintf("%x", holder.APIKey))
	assert.Equal(t, "(unset)", holder.APIKey.LogValue().String())
	assert.Contains(t, fmt.Sprintf("%+v", holder), "APIKey:(unset)")

	out, err := json.Marshal(holder)
	require.NoError(t, err)
	assert.JSONEq(t, `{"Name":"","APIKey":"(unset)"}`, string(out))

	assert.Equal(t, redacted, config.NewSecret("").String(), "set, even to an empty value: REDACTED")
}

func TestReadSecret(t *testing.T) {
	t.Parallel()

	const name = "SVC_API_KEY"

	testcases := []struct {
		label    string
		env      map[string]string
		required bool
		want     string // revealed value; empty: unset
		errText  string // expected substring of the error; empty: no error
	}{
		{label: "should read a set secret", env: map[string]string{name: secretValue}, required: true, want: secretValue},
		{
			label: "should drop a trailing newline (echo, mounted files)",
			env:   map[string]string{name: secretValue + "\n"},
			want:  secretValue,
		},
		{label: "should drop a trailing CRLF", env: map[string]string{name: secretValue + "\r\n"}, want: secretValue},
		{
			label: "should keep spaces, which can be part of a secret",
			env:   map[string]string{name: " " + secretValue + " "},
			want:  " " + secretValue + " ",
		},
		{
			label:    "should reject a required secret that is only a newline",
			env:      map[string]string{name: "\n"},
			required: true,
			errText:  name + " or " + name + "_FILE is required",
		},
		{label: "should leave an optional secret unset", required: false},
		{
			label:    "should reject a missing required secret",
			required: true,
			errText:  name + " or " + name + "_FILE is required",
		},
		{
			label:    "should reject a blank required secret",
			env:      map[string]string{name: "  "},
			required: true,
			errText:  name + " or " + name + "_FILE is required",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			var secret config.Secret

			err := config.ReadSecret(env(tc.env), name, tc.required, &secret)

			if tc.errText == "" {
				require.NoError(t, err)
				assert.Equal(t, tc.want, secret.Reveal())

				return
			}

			require.ErrorIs(t, err, config.ErrInvalid)
			assert.Contains(t, err.Error(), tc.errText)
			assert.Contains(t, err.Error(), "environment only")
			assert.False(t, secret.IsSet())
		})
	}
}

func TestReadSecret_file(t *testing.T) {
	t.Parallel()

	const (
		name     = "SVC_API_KEY"
		fileName = name + "_FILE"
		maxSize  = 64 << 10 // the limit documented for secret files
	)

	dir := t.TempDir()

	// file writes content to a new file in dir and returns its path.
	file := func(base, content string) string {
		path := filepath.Join(dir, base)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

		return path
	}

	testcases := []struct {
		label    string
		env      map[string]string
		required bool
		want     string // revealed value; empty: unset
		errText  string // expected substring of the error; empty: no error
	}{
		{
			label:    "should read the secret from the file",
			env:      map[string]string{fileName: file("plain", secretValue)},
			required: true,
			want:     secretValue,
		},
		{
			label: "should drop the file's trailing newline",
			env:   map[string]string{fileName: file("newline", secretValue+"\r\n")},
			want:  secretValue,
		},
		{
			label: "should read the file when the variable is set but empty",
			env:   map[string]string{name: "", fileName: file("empty-var", secretValue)},
			want:  secretValue,
		},
		{
			label: "should accept a file of exactly the size limit",
			env:   map[string]string{fileName: file("max", strings.Repeat("k", maxSize))},
			want:  strings.Repeat("k", maxSize),
		},
		{
			label:   "should reject both the variable and the file",
			env:     map[string]string{name: secretValue, fileName: file("both", secretValue)},
			errText: name + " and " + fileName + " are both set",
		},
		{
			label:   "should reject a missing file, even for an optional secret",
			env:     map[string]string{fileName: filepath.Join(dir, "missing")},
			errText: fileName + `="` + filepath.Join(dir, "missing") + `": no such file or directory`,
		},
		{
			label:   "should reject a file that cannot be read",
			env:     map[string]string{fileName: dir}, // a directory: unreadable for any user, root included
			errText: fileName + `="` + dir + `": is a directory`,
		},
		{
			label:   "should reject a file larger than the limit",
			env:     map[string]string{fileName: file("large", secretValue+strings.Repeat("k", maxSize))},
			errText: `": too large: over 64 KiB`,
		},
		{
			label:    "should reject a blank file for a required secret",
			env:      map[string]string{fileName: file("blank-required", "\n")},
			required: true,
			errText:  name + " or " + fileName + " is required",
		},
		{
			label: "should leave an optional secret unset when its file is blank",
			env:   map[string]string{fileName: file("blank-optional", "")},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			var secret config.Secret

			err := config.ReadSecret(env(tc.env), name, tc.required, &secret)

			if tc.errText == "" {
				require.NoError(t, err)
				assert.Equal(t, tc.want, secret.Reveal())

				return
			}

			require.ErrorIs(t, err, config.ErrInvalid)
			assert.Contains(t, err.Error(), tc.errText)
			assert.NotContains(t, err.Error(), secretValue, "an error never shows the secret")
			assert.LessOrEqual(t, strings.Count(err.Error(), tc.env[fileName]), 1, "the path is named once")
			assert.False(t, secret.IsSet())
		})
	}
}
