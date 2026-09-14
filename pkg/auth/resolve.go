package auth

import (
	"context"
	"strings"
	"time"

	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
)

// ResolveInput is the fully-merged auth input (flags + env + metadata) for
// selecting a TokenSource.
type ResolveInput struct {
	StaticToken  string                                    // --token / OPENSHELL_TOKEN
	ClientSecret func(ctx context.Context) (string, error) // nil when no secret available

	Issuer   string // flag/env overrides; empty = take from metadata
	ClientID string
	Audience string
	Scopes   []string

	Gateway         *gatewayconfig.Resolved // may be nil (endpoint-only)
	GatewayEndpoint string                  // for /auth/oidc-config fallback

	// OIDCConfigFetcher fetches {issuer, audience} from GET <endpoint>/auth/oidc-config.
	// nil disables the fallback.
	OIDCConfigFetcher func(ctx context.Context, endpoint string) (issuer, audience string, err error)

	// TokenFS / TokenWriter back the on-disk bundle source and its refresh
	// write-back. TokenFS is the gateway's sub-FS (r.FS).
	Refresher RefreshTokenExchanger // nil ok

	// TokenWriter persists a refreshed on-disk bundle (Rust CLI schema). nil
	// disables write-back (the refreshed token is still returned, just not saved).
	TokenWriter Writer // nil ok

	Clock func() time.Time
}

// Resolve picks a TokenSource per §5.2:
//  1. static token wins;
//  2. client secret present → client-credentials (overrides, then metadata, then
//     /auth/oidc-config fallback for issuer/audience);
//  3. no secret, gateway resolved, auth_mode oidc → on-disk bundle;
//  4. auth_mode none/plaintext → NoAuth;
//  5. auth_mode unset/mtls → NoAuth (gateway layer enforces the mTLS triple);
//  6. cloudflare_jwt → ErrUnsupportedAuthMode.
func Resolve(ctx context.Context, in ResolveInput, ex Exchanger) (TokenSource, error) {
	clock := in.Clock
	if clock == nil {
		clock = time.Now
	}

	if in.StaticToken != "" {
		return NewStaticSource(in.StaticToken, clock), nil
	}

	if in.ClientSecret != nil {
		return in.resolveClientCredentials(ctx, ex, clock)
	}

	mode := gatewayconfig.AuthModeUnset
	if in.Gateway != nil {
		mode = in.Gateway.Metadata.AuthMode
	}

	switch mode {
	case gatewayconfig.AuthModeOIDC:
		if in.Gateway == nil || in.Gateway.FS == nil {
			return nil, &ErrOIDCConfigMissing{Checked: []string{"oidc_token.json (no resolved gateway)"}}
		}
		gwName := ""
		if in.Gateway != nil {
			gwName = in.Gateway.Name
		}
		return NewDiskBundleSource(in.Gateway.FS, clock,
			WithRefresher(in.Refresher, in.TokenWriter),
			WithBundleGatewayName(gwName)), nil
	case gatewayconfig.AuthModeNone, gatewayconfig.AuthModePlaintext:
		return NewNoAuthSource(string(mode)), nil
	case gatewayconfig.AuthModeUnset, gatewayconfig.AuthModeMTLS:
		return NewNoAuthSource("mtls"), nil
	case gatewayconfig.AuthModeCloudflareJWT:
		return nil, &ErrUnsupportedAuthMode{Mode: string(mode)}
	default:
		return nil, &ErrUnsupportedAuthMode{Mode: string(mode)}
	}
}

func (in ResolveInput) resolveClientCredentials(ctx context.Context, ex Exchanger, clock func() time.Time) (TokenSource, error) {
	cfg := ClientCredentialsConfig{
		Issuer:   in.Issuer,
		ClientID: in.ClientID,
		Audience: in.Audience,
		Scopes:   in.Scopes,
		Secret:   in.ClientSecret,
	}

	// Fill from metadata where overrides are empty.
	if in.Gateway != nil {
		m := in.Gateway.Metadata
		if cfg.Issuer == "" && m.OIDCIssuer != nil {
			cfg.Issuer = *m.OIDCIssuer
		}
		if cfg.ClientID == "" {
			cfg.ClientID = m.OIDCClientIDOrDefault()
		}
		if cfg.Audience == "" && m.OIDCAudience != nil {
			cfg.Audience = *m.OIDCAudience
		}
		if len(cfg.Scopes) == 0 && m.OIDCScopes != nil {
			cfg.Scopes = strings.Fields(*m.OIDCScopes)
		}
	} else if cfg.ClientID == "" {
		cfg.ClientID = "openshell-cli"
	}

	// /auth/oidc-config fallback for issuer/audience when still missing.
	if (cfg.Issuer == "" || cfg.Audience == "") && in.OIDCConfigFetcher != nil && in.GatewayEndpoint != "" {
		if iss, aud, err := in.OIDCConfigFetcher(ctx, in.GatewayEndpoint); err == nil {
			if cfg.Issuer == "" {
				cfg.Issuer = iss
			}
			if cfg.Audience == "" {
				cfg.Audience = aud
			}
		}
	}

	if cfg.Issuer == "" {
		return nil, &ErrOIDCConfigMissing{Checked: []string{
			"--oidc-issuer", "metadata.oidc_issuer", "/auth/oidc-config",
		}}
	}

	gwName := ""
	if in.Gateway != nil {
		gwName = in.Gateway.Name
	}
	return NewClientCredentialsSource(cfg, ex, WithClock(clock), WithGatewayName(gwName)), nil
}
