package auth

import (
	"context"
	"time"

	oidc "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/oidc"
)

// sdkExchanger is the production Exchanger. It delegates OIDC discovery,
// issuer-match, HTTPS/loopback enforcement, and form encoding to the SDK's
// oidc.ClientCredentials, deriving expiresIn from the returned token's Expiry.
type sdkExchanger struct{}

// NewSDKExchanger returns the production Exchanger backed by the OpenShell SDK.
func NewSDKExchanger() Exchanger { return sdkExchanger{} }

// Exchange runs an OIDC client-credentials grant via the SDK.
func (sdkExchanger) Exchange(ctx context.Context, cfg ClientCredentialsConfig) (string, time.Duration, error) {
	opts := []oidc.LoginOption{
		oidc.WithIssuer(cfg.Issuer),
		oidc.WithClientID(cfg.ClientID),
	}
	if cfg.Secret != nil {
		opts = append(opts, oidc.WithClientSecretProvider(cfg.Secret))
	}
	if cfg.Audience != "" {
		opts = append(opts, oidc.WithAudience(cfg.Audience))
	}
	if len(cfg.Scopes) > 0 {
		opts = append(opts, oidc.WithScopes(cfg.Scopes...))
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	opts = append(opts, oidc.WithTimeout(timeout))

	tok, err := oidc.ClientCredentials(ctx, opts...)
	if err != nil {
		return "", 0, err
	}
	if tok.Expiry.IsZero() {
		return "", 0, ErrNoExpiry
	}
	expiresIn := time.Until(tok.Expiry)
	return tok.AccessToken, expiresIn, nil
}
