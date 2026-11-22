package config

import (
	"fmt"
	"log/slog"
	"strings"
)

// redacted is what a Secret shows instead of its value.
const redacted = "REDACTED"

// Secret holds a sensitive setting: an API key, a token, a password.
// Printing it (any fmt verb), logging it (slog) or encoding it (JSON, text)
// shows REDACTED; Reveal returns the value, for the one place that sends it.
//
// The value sits behind an unexported pointer: even where fmt bypasses the
// methods (an unexported field of a printed struct), it shows an address,
// never the value.
//
// Secrets are environment-only settings, read with readSecret: flags show up
// in ps and in shell history.
type Secret struct {
	value *string
}

// NewSecret returns a Secret holding v.
func NewSecret(v string) Secret {
	return Secret{value: &v}
}

// Reveal returns the value. Call it only where the value is sent (an
// Authorization header, a connection string), never to print or log it.
func (s Secret) Reveal() string {
	if s.value == nil {
		return ""
	}

	return *s.value
}

// IsSet reports whether the secret holds a value.
func (s Secret) IsSet() bool { return s.value != nil }

// String implements fmt.Stringer.
func (Secret) String() string { return redacted }

// GoString implements fmt.GoStringer (%#v).
func (Secret) GoString() string { return redacted }

// Format implements fmt.Formatter, so every verb (%x, %q, %d…) prints
// REDACTED too, not only %v and %s.
func (Secret) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(redacted)) }

// LogValue implements slog.LogValuer.
func (Secret) LogValue() slog.Value { return slog.StringValue(redacted) }

// MarshalText implements encoding.TextMarshaler, which encoding/json uses too.
func (Secret) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// readSecret reads the environment-only setting name into dst. A required
// secret that is missing or blank is an error wrapping ErrInvalid (a usage
// error, exit 2, reported with every other invalid value), so a deployment
// without it fails at startup, not on the first request. The value never
// appears in an error.
func readSecret(getenv func(string) string, name string, required bool, dst *Secret) error {
	v := getenv(name)
	if strings.TrimSpace(v) == "" {
		if required {
			return fmt.Errorf("%w: %s is required (environment only, never a flag)", ErrInvalid, name)
		}

		return nil
	}

	*dst = NewSecret(v)

	return nil
}
