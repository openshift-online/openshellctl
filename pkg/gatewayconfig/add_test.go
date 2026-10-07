package gatewayconfig

import (
	"errors"
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
