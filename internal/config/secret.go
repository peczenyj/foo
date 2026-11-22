package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"
)

// What a Secret shows instead of its value: redacted when it holds one,
// unset when it does not, so a config dump tells them apart.
const (
	redacted = "REDACTED"
	unset    = "(unset)"
)

const (
	// fileSuffix names the variable that holds a path to the secret instead
	// of the secret itself: FOO_API_KEY_FILE=/run/secrets/api_key.
	fileSuffix = "_FILE"
	// maxSecretFileKiB caps a secret file: a misconfigured path to a log or
	// a binary fails clearly instead of loading megabytes into memory. The
	// error message is built from it, so the two never disagree.
	maxSecretFileKiB  = 64
	maxSecretFileSize = maxSecretFileKiB << 10
)

var errSecretFileTooLarge = errors.New("too large")

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

// readSecret reads the environment-only setting name into dst, from the
// variable itself or, with the _FILE convention of Docker and Kubernetes
// secrets, from the file that name+"_FILE" points to (read once, at most
// maxSecretFileKiB KiB; the value then stays out of the process environment).
//
// Every failure is an error wrapping ErrInvalid (a usage error, exit 2,
// reported with every other invalid value), so a broken deployment fails at
// startup, not on the first request: both variables set, a file that cannot
// be read (even for an optional secret: setting _FILE shows intent) or is too
// large, and a required secret that is missing or blank. Errors name the
// variable and the path, never the value.
//
// Trailing line endings are dropped (echo and files often leave one, and the
// resulting authentication failure is hard to debug, since the value never
// shows); other spaces can be part of a secret and are kept.
func readSecret(getenv func(string) string, name string, required bool, dst *Secret) error {
	fileName := name + fileSuffix
	v, path := getenv(name), getenv(fileName)

	switch {
	case v != "" && path != "":
		return fmt.Errorf("%w: %s and %s are both set: set only one", ErrInvalid, name, fileName)
	case path != "":
		content, err := readSecretFile(path)
		if err != nil {
			return fmt.Errorf("%w: %s=%q: %w", ErrInvalid, fileName, path, err)
		}

		v = content
	}

	v = strings.TrimRight(v, "\r\n")
	if strings.TrimSpace(v) == "" {
		if required {
			return fmt.Errorf("%w: %s or %s is required (environment only, never a flag)", ErrInvalid, name, fileName)
		}

		return nil
	}

	*dst = NewSecret(v)

	return nil
}

// readSecretFile returns the content of the secret file at path, at most
// maxSecretFileSize bytes. Its errors never include the content, nor the
// path (the caller names it once).
func readSecretFile(path string) (string, error) {
	//nolint:gosec // G304: the path is the operator's own setting (<NAME>_FILE), by design.
	f, err := os.Open(path)
	if err != nil {
		return "", withoutPath(err)
	}

	defer func() { _ = f.Close() }()

	// One byte more than the limit tells "exactly the limit" from "larger".
	content, err := io.ReadAll(io.LimitReader(f, maxSecretFileSize+1))
	if err != nil {
		return "", withoutPath(err)
	}

	if len(content) > maxSecretFileSize {
		return "", fmt.Errorf("%w: over %d KiB", errSecretFileTooLarge, maxSecretFileKiB)
	}

	return string(content), nil
}

// withoutPath returns the cause of a *fs.PathError ("no such file or
// directory", "is a directory"), whose message would repeat the path.
func withoutPath(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		err = pathErr.Err
	}

	return err
}
