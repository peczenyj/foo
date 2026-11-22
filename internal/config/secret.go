package config

import (
	"fmt"
	"log/slog"
	"strings"
)

// What a Secret shows instead of its value: redacted when it holds one,
// unset when it does not, so a config dump tells them apart.
const (
	redacted = "REDACTED"
	unset    = "(unset)"
)

// Secret holds a sensitive setting: an API key, a token, a password.
// Printing it (any fmt verb), logging it (slog) or encoding it (JSON, text)
// shows REDACTED, or (unset) when it holds no value; Reveal returns the
// value, for the one place that sends it.
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
func (s Secret) String() string {
	if s.IsSet() {
		return redacted
	}

	return unset
}

// GoString implements fmt.GoStringer, for callers that use it directly (fmt
// itself calls Format, even for %#v).
func (s Secret) GoString() string { return s.String() }

// Format implements fmt.Formatter, so every verb (%x, %q, %d…) prints what
// String does (REDACTED, or (unset) without a value), not only %v and %s.
func (s Secret) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(s.String())) }

// LogValue implements slog.LogValuer.
func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }

// MarshalText implements encoding.TextMarshaler, which encoding/json uses too.
func (s Secret) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// readSecret reads the environment-only setting name into dst. Trailing line
// endings are dropped (a value written with echo or read from a file often
// ends with one, and the resulting authentication failure is hard to debug,
// since the value never shows); other spaces can be part of a secret and are
// kept. A required secret that is missing or blank is an error wrapping
// ErrInvalid (a usage error, exit 2, reported with every other invalid
// value), so a deployment without it fails at startup, not on the first
// request. The value never appears in an error.
func readSecret(getenv func(string) string, name string, required bool, dst *Secret) error {
	v := strings.TrimRight(getenv(name), "\r\n")
	if strings.TrimSpace(v) == "" {
		if required {
			return fmt.Errorf("%w: %s is required (environment only, never a flag)", ErrInvalid, name)
		}

		return nil
	}

	*dst = NewSecret(v)

	return nil
}
