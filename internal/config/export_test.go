package config

// ReadSecret exposes readSecret to the tests: foo itself declares no secret.
func ReadSecret(getenv func(string) string, name string, required bool, dst *Secret) error {
	return readSecret(getenv, name, required, dst)
}
