package config_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
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
			errText:  name + " is required",
		},
		{label: "should leave an optional secret unset", required: false},
		{label: "should reject a missing required secret", required: true, errText: name + " is required"},
		{
			label:    "should reject a blank required secret",
			env:      map[string]string{name: "  "},
			required: true,
			errText:  name + " is required",
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
