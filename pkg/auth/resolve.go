package auth

import (
	"context"
	"fmt"
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

	// TLSPresent reports whether mTLS material (mtls/ca.crt, at minimum) is
	// present for the resolved gateway — the caller is expected to compute
	// this via gatewayconfig.TLSMaterialFor(Gateway).Present (see
	// internal/cli/authwiring.go's resolveTokenSource). It is what lets
	// Resolve tell a genuinely no-auth-needed mTLS gateway apart from a
	// gateway (or no gateway at all) with literally nothing configured —
	// see rule 5/6 below.
	TLSPresent bool

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
//  4. auth_mode none/plaintext → NoAuth (an explicit, resolved declaration
//     that no auth is needed);
//  5. auth_mode unset/mtls, TLSPresent → NoAuth (a real, resolved mTLS
//     gateway; the dial layer enforces the cert triple, this package's job
//     is just "no bearer token needed");
//  6. auth_mode unset/mtls, NOT TLSPresent → ErrNoCredentials: nothing at
//     all was configured (no static token, no secret, no disk bundle, no
//     TLS material) — this is the "truly nothing to authenticate with" case
//     that previously, incorrectly, still returned a working-looking
//     NoAuthSource;
//  7. cloudflare_jwt → ErrUnsupportedAuthMode.
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
		if in.TLSPresent {
			return NewNoAuthSource("mtls"), nil
		}
		return nil, &ErrNoCredentials{Endpoint: in.GatewayEndpoint, Checked: in.noCredentialsChecked()}
	case gatewayconfig.AuthModeCloudflareJWT:
		return nil, &ErrUnsupportedAuthMode{Mode: string(mode)}
	default:
		return nil, &ErrUnsupportedAuthMode{Mode: string(mode)}
	}
}

// noCredentialsChecked lists, in resolution order, every place Resolve
// inspected before concluding nothing was configured — so ErrNoCredentials's
// message tells the user exactly where to look instead of making them guess.
func (in ResolveInput) noCredentialsChecked() []string {
	metadataPath, mtlsPath := "gateways/<name>/metadata.json", "gateways/<name>/mtls/"
	if in.Gateway != nil && in.Gateway.Name != "" {
		metadataPath = fmt.Sprintf("gateways/%s/metadata.json", in.Gateway.Name)
		mtlsPath = fmt.Sprintf("gateways/%s/mtls/", in.Gateway.Name)
	}
	return []string{
		"--token / OPENSHELL_TOKEN",
		"OPENSHELL_OIDC_CLIENT_SECRET (or --client-secret-file)",
		metadataPath,
		mtlsPath,
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
