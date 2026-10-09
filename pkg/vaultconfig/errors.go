// Package vaultconfig lets openshellctl source its gateway/OIDC auth config
// from a Vault KV secret, as a third config source alongside flags and
// OPENSHELL_* env vars. See docs/plans/0003-vault-auth-source-plan.md.
package vaultconfig

import "fmt"

// ErrNoToken is returned when neither VAULT_TOKEN nor ~/.vault-token yields a
// usable Vault token. openshellctl never performs `vault login` itself — it
// expects a session to already exist.
type ErrNoToken struct{}

func (e *ErrNoToken) Error() string {
	return "no Vault token found (checked $VAULT_TOKEN and ~/.vault-token) — run `vault login` first " +
		"(custom token helpers are not supported)"
}

// ErrVaultAddrNotSet is returned when the Vault auth source is activated
// (--vault-kv-mount/--vault-kv-path set) but VAULT_ADDR is empty. Without this
// check, the Vault SDK silently falls back to https://127.0.0.1:8200,
// producing a confusing connection-refused error instead of a clear one.
type ErrVaultAddrNotSet struct{}

func (e *ErrVaultAddrNotSet) Error() string {
	return "VAULT_ADDR is not set"
}

// ErrForbidden is returned when Vault responds 403 to the KV read — the
// caller is authenticated but not authorized to read the configured secret.
type ErrForbidden struct {
	Mount string
	Path  string
}

func (e *ErrForbidden) Error() string {
	return fmt.Sprintf("permission denied reading vault secret %s/%s", e.Mount, e.Path)
}

// ErrSecretNotFound is returned when the configured Vault secret does not
// exist at mount/path.
type ErrSecretNotFound struct {
	Mount string
	Path  string
}

func (e *ErrSecretNotFound) Error() string {
	return fmt.Sprintf("vault secret not found at %s/%s", e.Mount, e.Path)
}

// ErrFieldNotString is returned by ExtractFields when a recognized field is
// present in the secret but its value is not a string.
type ErrFieldNotString struct {
	Field string
}

func (e *ErrFieldNotString) Error() string {
	return fmt.Sprintf("vault secret field %q is not a string", e.Field)
}
