package auth

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"
	"time"

	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
)

func ptr(s string) *string { return &s }

func resolvedGW(mode gatewayconfig.AuthMode, m gatewayconfig.Metadata) *gatewayconfig.Resolved {
	m.AuthMode = mode
	return &gatewayconfig.Resolved{
		Name:     "rosa",
		Metadata: m,
		FS:       fstest.MapFS{},
	}
}

func TestResolve_StaticWins(t *testing.T) {
	in := ResolveInput{
		StaticToken:  "tok",
		ClientSecret: func(context.Context) (string, error) { return "s", nil },
		Gateway:      resolvedGW(gatewayconfig.AuthModeOIDC, gatewayconfig.Metadata{OIDCIssuer: ptr("https://i")}),
	}
	src, err := Resolve(context.Background(), in, &fakeExchanger{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := src.(*StaticSource); !ok {
		t.Errorf("got %T, want *StaticSource", src)
	}
}

func TestResolve_SecretGivesClientCredentials(t *testing.T) {
	in := ResolveInput{
		ClientSecret: func(context.Context) (string, error) { return "s", nil },
		Gateway: resolvedGW(gatewayconfig.AuthModeOIDC, gatewayconfig.Metadata{
			OIDCIssuer: ptr("https://issuer"),
		}),
	}
	src, err := Resolve(context.Background(), in, &fakeExchanger{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := src.(*ClientCredentialsSource); !ok {
		t.Errorf("got %T, want *ClientCredentialsSource", src)
	}
}

func TestResolve_ClientCredentialsUsesMetadataDefaults(t *testing.T) {
	in := ResolveInput{
		ClientSecret: func(context.Context) (string, error) { return "s", nil },
		Gateway: resolvedGW(gatewayconfig.AuthModeOIDC, gatewayconfig.Metadata{
			OIDCIssuer: ptr("https://issuer"),
			OIDCScopes: ptr("openid profile email"),
		}),
	}
	src, err := Resolve(context.Background(), in, &fakeExchanger{})
	if err != nil {
		t.Fatal(err)
	}
	cc := src.(*ClientCredentialsSource)
	if cc.cfg.Issuer != "https://issuer" {
		t.Errorf("issuer = %q", cc.cfg.Issuer)
	}
	if cc.cfg.ClientID != "openshell-cli" {
		t.Errorf("clientID default = %q, want openshell-cli", cc.cfg.ClientID)
	}
	if len(cc.cfg.Scopes) != 3 {
		t.Errorf("scopes = %v, want 3 split on whitespace", cc.cfg.Scopes)
	}
}

func TestResolve_OIDCConfigFallback(t *testing.T) {
	fetched := false
	in := ResolveInput{
		ClientSecret:    func(context.Context) (string, error) { return "s", nil },
		GatewayEndpoint: "https://gw",
		OIDCConfigFetcher: func(context.Context, string) (string, string, error) {
			fetched = true
			return "https://discovered", "openshell-cli", nil
		},
		// no gateway metadata → issuer must come from the fallback
	}
	src, err := Resolve(context.Background(), in, &fakeExchanger{})
	if err != nil {
		t.Fatal(err)
	}
	if !fetched {
		t.Error("expected the oidc-config fallback to be consulted")
	}
	cc := src.(*ClientCredentialsSource)
	if cc.cfg.Issuer != "https://discovered" {
		t.Errorf("issuer = %q, want discovered", cc.cfg.Issuer)
	}
}

func TestResolve_MissingIssuerErrors(t *testing.T) {
	in := ResolveInput{
		ClientSecret: func(context.Context) (string, error) { return "s", nil },
		// no metadata, no fetcher
	}
	_, err := Resolve(context.Background(), in, &fakeExchanger{})
	var missing *ErrOIDCConfigMissing
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want ErrOIDCConfigMissing", err)
	}
}

func TestResolve_AuthModeMatrix(t *testing.T) {
	tests := []struct {
		mode       gatewayconfig.AuthMode
		tlsPresent bool
		wantErr    bool
		check      func(TokenSource) bool
	}{
		{gatewayconfig.AuthModeOIDC, false, false, func(s TokenSource) bool { _, ok := s.(*DiskBundleSource); return ok }},
		{gatewayconfig.AuthModeNone, false, false, func(s TokenSource) bool { _, ok := s.(*NoAuthSource); return ok }},
		{gatewayconfig.AuthModePlaintext, false, false, func(s TokenSource) bool { _, ok := s.(*NoAuthSource); return ok }},
		// MTLS/Unset with TLS material present are legitimate, resolved mTLS
		// gateways — the dial layer enforces the cert triple, so pkg/auth
		// correctly reports "no bearer token needed" via NoAuthSource.
		{gatewayconfig.AuthModeMTLS, true, false, func(s TokenSource) bool { _, ok := s.(*NoAuthSource); return ok }},
		{gatewayconfig.AuthModeUnset, true, false, func(s TokenSource) bool { _, ok := s.(*NoAuthSource); return ok }},
		{gatewayconfig.AuthModeCloudflareJWT, false, true, nil},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			in := ResolveInput{Gateway: resolvedGW(tt.mode, gatewayconfig.Metadata{}), TLSPresent: tt.tlsPresent}
			src, err := Resolve(context.Background(), in, &fakeExchanger{})
			if tt.wantErr {
				var unsupported *ErrUnsupportedAuthMode
				if !errors.As(err, &unsupported) {
					t.Fatalf("err = %v, want ErrUnsupportedAuthMode", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !tt.check(src) {
				t.Errorf("unexpected source type %T for mode %q", src, tt.mode)
			}
		})
	}
}

// TestResolve_NoCredentialsMatrix is the table test over the full
// no-credentials input matrix: StaticToken x ClientSecret x Gateway x
// TLSPresent. ErrNoCredentials fires only when every one of static token,
// client secret, a resolved OIDC disk bundle, and TLS material is absent —
// i.e. genuinely nothing was configured, not merely "no auth needed".
func TestResolve_NoCredentialsMatrix(t *testing.T) {
	tests := []struct {
		name       string
		in         ResolveInput
		wantNoCred bool
	}{
		{
			name:       "nothing configured at all (Gateway nil)",
			in:         ResolveInput{},
			wantNoCred: true,
		},
		{
			name:       "TLSPresent is trusted as given, independent of Gateway (the caller, resolveTokenSource, only ever sets it true when Gateway is also resolved — Resolve itself just trusts the signal)",
			in:         ResolveInput{TLSPresent: true},
			wantNoCred: false,
		},
		{
			name:       "Gateway resolved, AuthModeUnset, no TLS material -> no credentials",
			in:         ResolveInput{Gateway: resolvedGW(gatewayconfig.AuthModeUnset, gatewayconfig.Metadata{})},
			wantNoCred: true,
		},
		{
			name:       "Gateway resolved, AuthModeMTLS, no TLS material -> no credentials",
			in:         ResolveInput{Gateway: resolvedGW(gatewayconfig.AuthModeMTLS, gatewayconfig.Metadata{})},
			wantNoCred: true,
		},
		{
			name:       "Gateway resolved, AuthModeUnset, TLS material present -> legitimate mTLS, not an error",
			in:         ResolveInput{Gateway: resolvedGW(gatewayconfig.AuthModeUnset, gatewayconfig.Metadata{}), TLSPresent: true},
			wantNoCred: false,
		},
		{
			name:       "static token present -> not an error even with nothing else",
			in:         ResolveInput{StaticToken: "tok"},
			wantNoCred: false,
		},
		{
			name:       "client secret present -> not an error even with nothing else",
			in:         ResolveInput{ClientSecret: func(context.Context) (string, error) { return "s", nil }, Gateway: resolvedGW(gatewayconfig.AuthModeOIDC, gatewayconfig.Metadata{OIDCIssuer: ptr("https://i")})},
			wantNoCred: false,
		},
		{
			name:       "Gateway resolved with AuthModeNone (explicit no-auth) -> legitimate, not an error, regardless of TLSPresent",
			in:         ResolveInput{Gateway: resolvedGW(gatewayconfig.AuthModeNone, gatewayconfig.Metadata{})},
			wantNoCred: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(context.Background(), tt.in, &fakeExchanger{})
			if tt.wantNoCred {
				if !errors.Is(err, ErrNoCredentials) {
					t.Fatalf("err = %v, want ErrNoCredentials", err)
				}
				return
			}
			if errors.Is(err, ErrNoCredentials) {
				t.Fatalf("unexpected ErrNoCredentials for input %+v", tt.in)
			}
		})
	}
}

func TestResolve_NoAuthTokenHasNoHeader(t *testing.T) {
	in := ResolveInput{Gateway: resolvedGW(gatewayconfig.AuthModePlaintext, gatewayconfig.Metadata{})}
	src, err := Resolve(context.Background(), in, &fakeExchanger{})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Source != SourceNone || tok.AccessToken != "" {
		t.Errorf("no-auth token = %+v, want empty SourceNone", tok)
	}
}

func TestResolve_DefaultClockUsed(t *testing.T) {
	// Ensures a nil clock does not panic.
	in := ResolveInput{StaticToken: makeJWT(t, map[string]any{"iss": "https://i", "exp": float64(time.Now().Add(time.Hour).Unix())})}
	if _, err := Resolve(context.Background(), in, &fakeExchanger{}); err != nil {
		t.Fatal(err)
	}
}
