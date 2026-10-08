package gatewayconfig

import (
	"errors"
	"strings"
	"testing"
)

func TestNewMetadata_ExplicitIssuer(t *testing.T) {
	m, err := NewMetadata(AddInput{
		Endpoint:     "https://gw.example.com:443",
		Name:         "my-gw",
		OIDCIssuer:   "https://issuer.example.com",
		OIDCClientID: "client-1",
		OIDCAudience: "aud-1",
	})
	if err != nil {
		t.Fatalf("NewMetadata: %v", err)
	}
	if m.Name != "my-gw" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.GatewayEndpoint != "https://gw.example.com:443" {
		t.Errorf("GatewayEndpoint = %q", m.GatewayEndpoint)
	}
	if !m.IsRemote {
		t.Error("IsRemote should be true")
	}
	if m.GatewayPort != 0 {
		t.Errorf("GatewayPort = %d, want 0", m.GatewayPort)
	}
	if m.AuthMode != AuthModeOIDC {
		t.Errorf("AuthMode = %q", m.AuthMode)
	}
	if m.OIDCIssuer == nil || *m.OIDCIssuer != "https://issuer.example.com" {
		t.Errorf("OIDCIssuer = %v", m.OIDCIssuer)
	}
	if m.OIDCClientID == nil || *m.OIDCClientID != "client-1" {
		t.Errorf("OIDCClientID = %v", m.OIDCClientID)
	}
	if m.OIDCAudience == nil || *m.OIDCAudience != "aud-1" {
		t.Errorf("OIDCAudience = %v", m.OIDCAudience)
	}
}

func TestNewMetadata_SchemePrepended(t *testing.T) {
	m, err := NewMetadata(AddInput{
		Endpoint:   "gw.example.com:443",
		OIDCIssuer: "https://issuer",
	})
	if err != nil {
		t.Fatalf("NewMetadata: %v", err)
	}
	if m.GatewayEndpoint != "https://gw.example.com:443" {
		t.Errorf("GatewayEndpoint = %q, want https:// prepended", m.GatewayEndpoint)
	}
}

func TestNewMetadata_NameDefaultedFromHost(t *testing.T) {
	m, err := NewMetadata(AddInput{
		Endpoint:   "https://gw-foo.example.com:443",
		OIDCIssuer: "https://issuer",
	})
	if err != nil {
		t.Fatalf("NewMetadata: %v", err)
	}
	if m.Name != "gw-foo.example.com" {
		t.Errorf("Name = %q, want the endpoint host", m.Name)
	}
}

func TestNewMetadata_ExplicitNameInvalid(t *testing.T) {
	_, err := NewMetadata(AddInput{
		Endpoint:   "https://gw.example.com",
		Name:       "has/slash",
		OIDCIssuer: "https://issuer",
	})
	var invalid *InvalidGatewayNameError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidGatewayNameError", err)
	}
}

func TestNewMetadata_UsesDiscoveredOIDC(t *testing.T) {
	m, err := NewMetadata(AddInput{
		Endpoint: "https://gw.example.com",
		Discovered: &DiscoveredOIDC{
			Issuer:   "https://discovered-issuer",
			Audience: "discovered-aud",
		},
	})
	if err != nil {
		t.Fatalf("NewMetadata: %v", err)
	}
	if m.OIDCIssuer == nil || *m.OIDCIssuer != "https://discovered-issuer" {
		t.Errorf("OIDCIssuer = %v, want discovered issuer", m.OIDCIssuer)
	}
	if m.OIDCAudience == nil || *m.OIDCAudience != "discovered-aud" {
		t.Errorf("OIDCAudience = %v, want discovered audience", m.OIDCAudience)
	}
}

func TestNewMetadata_ExplicitIssuerWinsOverDiscovered(t *testing.T) {
	m, err := NewMetadata(AddInput{
		Endpoint:   "https://gw.example.com",
		OIDCIssuer: "https://explicit-issuer",
		Discovered: &DiscoveredOIDC{Issuer: "https://discovered-issuer"},
	})
	if err != nil {
		t.Fatalf("NewMetadata: %v", err)
	}
	if *m.OIDCIssuer != "https://explicit-issuer" {
		t.Errorf("OIDCIssuer = %q, want the explicit override to win", *m.OIDCIssuer)
	}
}

func TestNewMetadata_NoIssuerNoDiscovery_ErrEdgeGatewayUnsupported(t *testing.T) {
	_, err := NewMetadata(AddInput{Endpoint: "https://gw.example.com"})
	var edge *EdgeGatewayUnsupportedError
	if !errors.As(err, &edge) {
		t.Fatalf("err = %v, want EdgeGatewayUnsupportedError", err)
	}
	if !errors.Is(err, ErrEdgeGatewayUnsupported) {
		t.Error("err should match ErrEdgeGatewayUnsupported via errors.Is")
	}
}

func TestNewMetadata_MTLSRequested_ErrMTLSUnsupported(t *testing.T) {
	_, err := NewMetadata(AddInput{
		Endpoint: "https://gw.example.com",
		AuthMode: AuthModeMTLS,
	})
	var mtls *MTLSUnsupportedError
	if !errors.As(err, &mtls) {
		t.Fatalf("err = %v, want MTLSUnsupportedError", err)
	}
	if !errors.Is(err, ErrMTLSUnsupported) {
		t.Error("err should match ErrMTLSUnsupported via errors.Is")
	}
}

func TestNewMetadata_InvalidEndpoint(t *testing.T) {
	_, err := NewMetadata(AddInput{Endpoint: "://not a url"})
	var invalid *InvalidEndpointError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidEndpointError", err)
	}
	if !errors.Is(err, ErrInvalidEndpoint) {
		t.Error("err should match ErrInvalidEndpoint via errors.Is")
	}
}

func TestNewMetadata_EmptyEndpoint(t *testing.T) {
	_, err := NewMetadata(AddInput{Endpoint: ""})
	var invalid *InvalidEndpointError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidEndpointError for empty endpoint", err)
	}
}

// TestNewMetadata_ValidatesWithNormalizeEndpoint confirms NewMetadata calls
// NormalizeEndpoint — the exact function FindByEndpoint will later use to
// match this gateway — as part of its own validation, coupling write-time
// validation to match-time validation so a registration can never succeed
// while producing an endpoint that's later unfindable by FindByEndpoint.
//
// Honesty note (raised in review): for every input known today,
// EnsureScheme's own url.Parse call already rejects anything
// NormalizeEndpoint would also reject (both parse the same way and require a
// non-empty host), so this specific input doesn't exercise a class of error
// EnsureScheme would otherwise miss — it pins the coupling as an invariant,
// not a presently-reachable new failure mode. If the two functions' parsing
// ever diverges, this is the test that would catch a registration slipping
// through with an endpoint NormalizeEndpoint can't later match.
func TestNewMetadata_ValidatesWithNormalizeEndpoint(t *testing.T) {
	_, err := NewMetadata(AddInput{
		Endpoint:   "https://gw.example.com:notaport",
		OIDCIssuer: "https://issuer",
	})
	var invalid *InvalidEndpointError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidEndpointError", err)
	}
}

// TestNewMetadata_StoresEndpointAsGiven_NotNormalized confirms the
// NormalizeEndpoint validation pass is purely a check — Metadata.
// GatewayEndpoint still stores EnsureScheme's result (scheme defaulted, but
// otherwise verbatim), never NormalizeEndpoint's canonical form (lower-cased,
// default port stripped). Storing the normalized form would make the
// metadata.json diverge from what the user/script actually typed for no
// functional benefit, since FindByEndpoint normalizes at compare time anyway.
func TestNewMetadata_StoresEndpointAsGiven_NotNormalized(t *testing.T) {
	m, err := NewMetadata(AddInput{
		Endpoint:   "HTTPS://GW.example.com:443/",
		OIDCIssuer: "https://issuer",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.GatewayEndpoint != "HTTPS://GW.example.com:443/" {
		t.Errorf("GatewayEndpoint = %q, want the endpoint stored verbatim (as EnsureScheme returns it)", m.GatewayEndpoint)
	}
}

func TestNewMetadata_ScopesSet(t *testing.T) {
	m, err := NewMetadata(AddInput{
		Endpoint:   "https://gw.example.com",
		OIDCIssuer: "https://issuer",
		OIDCScopes: "openid profile",
	})
	if err != nil {
		t.Fatalf("NewMetadata: %v", err)
	}
	if m.OIDCScopes == nil || *m.OIDCScopes != "openid profile" {
		t.Errorf("OIDCScopes = %v", m.OIDCScopes)
	}
}

// TestNewMetadata_ClientIDDefaultedWhenEmpty confirms metadata.json always
// carries an explicit oidc_client_id (matching Metadata.OIDCClientIDOrDefault's
// "openshell-cli" default) rather than omitting the field when --oidc-client-id
// wasn't given — so the written registration is self-describing, and a caller
// reading it back doesn't need to separately know the default.
func TestNewMetadata_ClientIDDefaultedWhenEmpty(t *testing.T) {
	m, err := NewMetadata(AddInput{Endpoint: "https://gw.example.com", OIDCIssuer: "https://issuer"})
	if err != nil {
		t.Fatalf("NewMetadata: %v", err)
	}
	if m.OIDCClientID == nil || *m.OIDCClientID != "openshell-cli" {
		t.Errorf("OIDCClientID = %v, want the defaulted \"openshell-cli\"", m.OIDCClientID)
	}
}

// TestNewMetadata_RejectsNonHTTPSEndpoint confirms NewMetadata refuses to
// register an OIDC gateway over a non-https endpoint: pkg/gateway's Dial
// rejects a plaintext (http://) endpoint carrying bearer auth with
// ErrPlaintextWithAuth, so writing auth_mode: oidc for one would produce an
// undialable registration.
func TestNewMetadata_RejectsNonHTTPSEndpoint(t *testing.T) {
	_, err := NewMetadata(AddInput{Endpoint: "http://gw.example.com", OIDCIssuer: "https://issuer"})
	var invalid *InvalidEndpointError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidEndpointError for a non-https endpoint", err)
	}
}

// TestNewMetadata_EdgeGatewayUnsupported_CarriesDiscoveryCause confirms a
// discovery-probe error (DNS failure, timeout, TLS error — anything other
// than a clean "not OIDC" signal) is surfaced in EdgeGatewayUnsupportedError
// rather than collapsed into the same generic message a real 404 gets, so a
// typo'd hostname doesn't get told "this looks like a non-OIDC gateway."
func TestNewMetadata_EdgeGatewayUnsupported_CarriesDiscoveryCause(t *testing.T) {
	cause := errors.New("dial tcp: lookup gw.example.com: no such host")
	_, err := NewMetadata(AddInput{Endpoint: "https://gw.example.com", DiscoveryErr: cause})
	var edge *EdgeGatewayUnsupportedError
	if !errors.As(err, &edge) {
		t.Fatalf("err = %v, want EdgeGatewayUnsupportedError", err)
	}
	if edge.Cause != cause {
		t.Errorf("Cause = %v, want the discovery error threaded through", edge.Cause)
	}
	if !strings.Contains(err.Error(), "no such host") {
		t.Errorf("message should include the discovery cause, got: %v", err)
	}
}
