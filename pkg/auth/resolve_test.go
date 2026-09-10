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
		mode    gatewayconfig.AuthMode
		wantErr bool
		check   func(TokenSource) bool
	}{
		{gatewayconfig.AuthModeOIDC, false, func(s TokenSource) bool { _, ok := s.(*DiskBundleSource); return ok }},
		{gatewayconfig.AuthModeNone, false, func(s TokenSource) bool { _, ok := s.(*NoAuthSource); return ok }},
		{gatewayconfig.AuthModePlaintext, false, func(s TokenSource) bool { _, ok := s.(*NoAuthSource); return ok }},
		{gatewayconfig.AuthModeMTLS, false, func(s TokenSource) bool { _, ok := s.(*NoAuthSource); return ok }},
		{gatewayconfig.AuthModeUnset, false, func(s TokenSource) bool { _, ok := s.(*NoAuthSource); return ok }},
		{gatewayconfig.AuthModeCloudflareJWT, true, nil},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			in := ResolveInput{Gateway: resolvedGW(tt.mode, gatewayconfig.Metadata{})}
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
