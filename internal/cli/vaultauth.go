package cli

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/viper"

	"github.com/openshift-online/openshellctl/pkg/vaultconfig"
)

// vaultDeps holds a test-injected VaultReader, mirroring cliDeps (deps.go) but
// kept separate: cliDeps is explicitly documented as read in exactly one
// place (resolveAuth), and applyVaultAuthSource is called from several
// call sites that don't go through resolveAuth at all (runLogin,
// runGatewayAdd, gateway logout, sandbox ssh-config) — overloading cliDeps
// here would make that doc comment wrong for every one of them.
type vaultDeps struct {
	Reader vaultconfig.VaultReader
}

type vaultDepsCtxKey struct{}

func withVaultDeps(ctx context.Context, d vaultDeps) context.Context {
	return context.WithValue(ctx, vaultDepsCtxKey{}, d)
}

func vaultDepsFrom(ctx context.Context) (vaultDeps, bool) {
	d, ok := ctx.Value(vaultDepsCtxKey{}).(vaultDeps)
	return d, ok
}

// vaultAuthOnce/vaultAuthResult memoize applyVaultAuthSource for the lifetime
// of the process. Without this, a single command whose flow reaches more than
// one of the call sites in applyVaultAuthSource's doc comment — e.g.
// `gateway add` with a Vault-sourced client secret, which calls it directly
// and then again via authenticateNewGateway -> resolveAuth -> resolveTokenSource
// — issues a second full Vault KV read for the exact same mount/path/prefix
// within one process. That's wasteful at best; at worst, with a single-use or
// response-wrapped Vault token (a common pattern for short-lived CI
// credentials — exactly what this feature targets), the first read consumes
// the token and the second one fails outright. The flags driving mount/path/
// prefix are parsed once per process and never change, so caching the result
// for the process lifetime is always correct, not just an optimization.
var (
	vaultAuthOnce   sync.Once
	vaultAuthResult error
)

// applyVaultAuthSource fills missing gateway/OIDC viper keys from a Vault KV
// secret, when --vault-kv-mount/--vault-kv-path (or their env vars) are set.
// It is a complete no-op otherwise.
//
// Called individually from every command that reads one of the affected
// viper keys (resolveTokenSource, runLogin, runGatewayAdd, gateway logout,
// sandbox ssh-config) rather than from root's PersistentPreRunE, so a command
// with no auth dependency at all (version, policy lint, ...) never needs a
// live Vault session just because OPENSHELL_VAULT_KV_MOUNT/PATH happen to be
// set in the environment.
func applyVaultAuthSource(ctx context.Context) error {
	vaultAuthOnce.Do(func() {
		vaultAuthResult = resolveVaultAuthSource(ctx)
	})
	return vaultAuthResult
}

func resolveVaultAuthSource(ctx context.Context) error {
	mount := viper.GetString("vault-kv-mount")
	path := viper.GetString("vault-kv-path")
	prefix := viper.GetString("vault-field-prefix")
	if mount == "" && path == "" {
		if prefix != "" {
			return &UsageError{Err: fmt.Errorf("--vault-field-prefix has no effect without --vault-kv-mount and --vault-kv-path")}
		}
		return nil
	}
	if mount == "" || path == "" {
		return &UsageError{Err: fmt.Errorf("--vault-kv-mount and --vault-kv-path must be set together")}
	}

	reader, err := vaultReaderFor(ctx)
	if err != nil {
		return err
	}

	data, err := reader.ReadSecret(ctx, mount, path)
	if err != nil {
		return err
	}

	fields, err := vaultconfig.ExtractFields(data, prefix)
	if err != nil {
		return err
	}

	overrides := vaultconfig.Overrides{
		Gateway:            viper.GetString("gateway") != "",
		GatewayEndpoint:    viper.GetString("gateway-endpoint") != "",
		OIDCClientID:       viper.GetString("oidc-client-id") != "",
		ClientSecretSource: viper.GetString("oidc.client_secret") != "" || viper.GetString("client-secret-file") != "",
	}
	for key, val := range vaultconfig.ToViperDefaults(fields, overrides) {
		viper.SetDefault(key, val)
	}
	return nil
}

// vaultReaderFor returns the context-injected VaultReader for tests, or
// builds the real Vault-API-backed one.
func vaultReaderFor(ctx context.Context) (vaultconfig.VaultReader, error) {
	if d, ok := vaultDepsFrom(ctx); ok && d.Reader != nil {
		return d.Reader, nil
	}
	return realVaultReader()
}

// realVaultReader is the production I/O shell: build a Vault client (failing
// fast if VAULT_ADDR is unset), resolve a token if the SDK didn't already
// pick one up from VAULT_TOKEN, and wrap the client as a VaultReader.
// openshellctl never performs `vault login` itself.
func realVaultReader() (vaultconfig.VaultReader, error) {
	client, err := vaultconfig.NewAPIClient(os.Getenv)
	if err != nil {
		return nil, err
	}
	if client.Token() == "" {
		token, err := vaultconfig.ResolveToken(vaultconfig.TokenEnv{
			Getenv:   os.Getenv,
			ReadFile: os.ReadFile,
			HomeDir:  os.UserHomeDir,
		})
		if err != nil {
			return nil, err
		}
		client.SetToken(token)
	}
	return vaultconfig.NewAPIReader(client), nil
}
