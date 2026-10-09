package vaultconfig

import (
	"path/filepath"
	"strings"
)

// TokenEnv is the injected environment ResolveToken reads from, so it never
// touches the real process environment or filesystem directly — callers wire
// a real TokenEnv (os.Getenv, os.ReadFile, os.UserHomeDir) at the edge.
type TokenEnv struct {
	Getenv   func(string) string
	ReadFile func(string) ([]byte, error)
	HomeDir  func() (string, error)
}

// ResolveToken resolves a Vault token the same way `vault login` leaves one
// behind, without ever performing a login itself: VAULT_TOKEN first, then the
// default token-helper file (~/.vault-token), trimmed of whitespace. Returns
// *ErrNoToken when neither yields a usable token.
func ResolveToken(env TokenEnv) (string, error) {
	if v := env.Getenv("VAULT_TOKEN"); v != "" {
		return v, nil
	}

	home, err := env.HomeDir()
	if err != nil {
		return "", &ErrNoToken{}
	}

	b, err := env.ReadFile(filepath.Join(home, ".vault-token"))
	if err != nil {
		return "", &ErrNoToken{}
	}

	token := strings.TrimSpace(string(b))
	if token == "" {
		return "", &ErrNoToken{}
	}
	return token, nil
}
