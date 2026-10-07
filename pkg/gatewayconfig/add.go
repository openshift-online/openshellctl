package gatewayconfig

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// DiscoveredOIDC is the result of a prior /auth/oidc-config probe against a
// gateway endpoint. It is plain data: the HTTP call itself happens in the CLI
// layer (internal/cli), keeping NewMetadata pure.
type DiscoveredOIDC struct {
	Issuer   string
	Audience string
}

// AddInput is the caller-gathered input for NewMetadata: CLI flags plus the
// result (if any) of an OIDC discovery probe. All fields are plain data; no
// I/O happens in this package for gateway add.
type AddInput struct {
	Endpoint     string // raw, as given on the CLI
	Name         string // "" -> derive from the endpoint host
	OIDCIssuer   string // explicit --oidc-issuer override; wins over Discovered
	OIDCClientID string
	OIDCAudience string
	OIDCScopes   string
	Discovered   *DiscoveredOIDC // result of a prior /auth/oidc-config probe; nil if not attempted or it failed
	AuthMode     AuthMode        // "" (default) infers OIDC via OIDCIssuer/Discovered; AuthModeMTLS is rejected
}

// NewMetadata builds the Metadata to write for `gateway add`. Pure: no I/O,
// no network. It normalizes the endpoint (defaulting the scheme to https://),
// defaults Name from the endpoint host when empty, and resolves the OIDC
// issuer/audience from an explicit override or a prior discovery result.
//
// gateway add only ever registers OIDC gateways (IsRemote: true, GatewayPort:
// 0 — matching the rosa-agent CronJob's own convention for a remote
// endpoint-addressed gateway, see metadata_marshal_test.go). mTLS gateway
// registration and non-OIDC ("edge") gateways are out of scope for this epic
// and return typed errors rather than being silently misregistered.
func NewMetadata(in AddInput) (Metadata, error) {
	if in.AuthMode == AuthModeMTLS {
		return Metadata{}, &MTLSUnsupportedError{}
	}

	endpoint, err := normalizeAddEndpoint(in.Endpoint)
	if err != nil {
		return Metadata{}, err
	}

	name := in.Name
	if name == "" {
		name = defaultNameFromEndpoint(endpoint)
	}
	if err := ValidateGatewayName(name); err != nil {
		return Metadata{}, err
	}

	issuer := in.OIDCIssuer
	audience := in.OIDCAudience
	if issuer == "" {
		if in.Discovered == nil || in.Discovered.Issuer == "" {
			return Metadata{}, &EdgeGatewayUnsupportedError{Endpoint: endpoint}
		}
		issuer = in.Discovered.Issuer
		if audience == "" {
			audience = in.Discovered.Audience
		}
	}

	m := Metadata{
		Name:            name,
		GatewayEndpoint: endpoint,
		IsRemote:        true,
		GatewayPort:     0,
		AuthMode:        AuthModeOIDC,
		OIDCIssuer:      &issuer,
	}
	if in.OIDCClientID != "" {
		clientID := in.OIDCClientID
		m.OIDCClientID = &clientID
	}
	if audience != "" {
		aud := audience
		m.OIDCAudience = &aud
	}
	if in.OIDCScopes != "" {
		scopes := in.OIDCScopes
		m.OIDCScopes = &scopes
	}
	return m, nil
}

// normalizeAddEndpoint defaults a missing scheme to https:// and validates
// the result parses as a URL with a non-empty host.
func normalizeAddEndpoint(endpoint string) (string, error) {
	if endpoint == "" {
		return "", &InvalidEndpointError{Endpoint: endpoint, Cause: errors.New("empty endpoint")}
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", &InvalidEndpointError{Endpoint: endpoint, Cause: err}
	}
	if u.Hostname() == "" {
		return "", &InvalidEndpointError{Endpoint: endpoint, Cause: fmt.Errorf("no host in endpoint")}
	}
	return endpoint, nil
}

// defaultNameFromEndpoint derives a gateway name from the endpoint's
// hostname (no port, no path) when --name is not given.
func defaultNameFromEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	return u.Hostname()
}
