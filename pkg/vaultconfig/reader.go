package vaultconfig

import (
	"context"
	"errors"
	"net/http"

	"github.com/hashicorp/vault/api"
)

// VaultReader reads a single KV v2 secret's data. It is the one network
// boundary for Vault access, so tests can use a fake instead of a real Vault
// server.
type VaultReader interface {
	ReadSecret(ctx context.Context, mount, path string) (map[string]any, error)
}

// NewAPIClient builds a Vault API client, failing fast with *ErrVaultAddrNotSet
// when VAULT_ADDR (per getenv) is empty, rather than letting the SDK silently
// default to https://127.0.0.1:8200 and fail later with a confusing
// connection-refused error. getenv is injected so this check is testable
// without touching the real environment; the underlying api.DefaultConfig()/
// api.NewClient() calls still read the real process environment for
// everything else (VAULT_CACERT, VAULT_NAMESPACE, ...) and perform no network
// I/O of their own — only an actual request does.
func NewAPIClient(getenv func(string) string) (*api.Client, error) {
	if getenv("VAULT_ADDR") == "" {
		return nil, &ErrVaultAddrNotSet{}
	}
	cfg := api.DefaultConfig()
	if cfg.Error != nil {
		return nil, cfg.Error
	}
	return api.NewClient(cfg)
}

// apiReader is the thin, real VaultReader implementation wrapping the official
// Vault SDK's KV v2 client.
type apiReader struct {
	client *api.Client
}

// NewAPIReader wraps client as a VaultReader backed by the KV v2 engine.
func NewAPIReader(client *api.Client) VaultReader {
	return &apiReader{client: client}
}

func (r *apiReader) ReadSecret(ctx context.Context, mount, path string) (map[string]any, error) {
	secret, err := r.client.KVv2(mount).Get(ctx, path)
	if err != nil {
		return nil, classifyVaultError(err, mount, path)
	}
	return secretData(secret, mount, path)
}

// secretData extracts a successful KV v2 read's data, treating a soft-deleted
// secret version the same as one that was never there at all: per the Vault
// SDK's own doc comment on KVv2.Get, `vault kv delete` (as opposed to `kv
// destroy`) returns no error but a nil Data field.
func secretData(secret *api.KVSecret, mount, path string) (map[string]any, error) {
	if secret == nil || secret.Data == nil {
		return nil, &ErrSecretNotFound{Mount: mount, Path: path}
	}
	return secret.Data, nil
}

// classifyVaultError maps the Vault SDK's own errors to this package's typed
// errors for the two cases openshellctl gives specific guidance for; every
// other error (network failures, 5xx, ...) passes through unchanged.
func classifyVaultError(err error, mount, path string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, api.ErrSecretNotFound) {
		return &ErrSecretNotFound{Mount: mount, Path: path}
	}
	var respErr *api.ResponseError
	if errors.As(err, &respErr) && respErr.StatusCode == http.StatusForbidden {
		return &ErrForbidden{Mount: mount, Path: path}
	}
	return err
}
