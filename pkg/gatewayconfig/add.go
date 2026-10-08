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
	Discovered   *DiscoveredOIDC // result of a successful /auth/oidc-config probe; nil if not attempted or it failed
	DiscoveryErr error           // the probe's own error, when Discovered is nil and a probe was attempted; threaded into EdgeGatewayUnsupportedError so a DNS/TLS/timeout failure isn't reported identically to a clean "not OIDC" 404
	AuthMode     AuthMode        // "" (default) infers OIDC via OIDCIssuer/Discovered; AuthModeMTLS is rejected
}

// NewMetadata builds the Metadata to write for `gateway add`. Pure: no I/O,
// no network. It normalizes the endpoint (defaulting the scheme to https://),
// defaults Name from the endpoint host when empty, and resolves the OIDC
// issuer/audience from an explicit override or a prior discovery result.
//
// gateway add only ever registers OIDC gateways over https (IsRemote: true,
// GatewayPort: 0 — matching the rosa-agent CronJob's own convention for a
// remote endpoint-addressed gateway, see metadata_marshal_test.go). A
// plaintext (http://) endpoint is rejected: pkg/gateway's Dial refuses a
// plaintext endpoint carrying bearer auth (ErrPlaintextWithAuth), so writing
// auth_mode: oidc for one would produce a registration that can never dial.
// mTLS gateway registration and non-OIDC ("edge") gateways are likewise out
// of scope for this epic and return typed errors rather than being silently
// misregistered.
func NewMetadata(in AddInput) (Metadata, error) {
	if in.AuthMode == AuthModeMTLS {
		return Metadata{}, &MTLSUnsupportedError{}
	}

	endpoint, err := EnsureScheme(in.Endpoint)
	if err != nil {
		return Metadata{}, err
	}
	// Scheme comparison is case-insensitive (URI schemes are, and
	// NormalizeEndpoint below already treats "HTTPS://" and "https://" as
	// the same gateway) — rejecting an uppercase scheme here while
	// FindByEndpoint later matches it anyway would be an inconsistent gap.
	if !strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		return Metadata{}, &InvalidEndpointError{
			Endpoint: endpoint,
			Cause:    fmt.Errorf("OIDC gateway registration requires https (got %q)", endpoint),
		}
	}
	// Validate against the exact same function FindByEndpoint will use to
	// match this gateway later, so a registration can never succeed while
	// being permanently unfindable by endpoint. The endpoint actually
	// stored (below) is still EnsureScheme's result, not this normalized
	// form — NormalizeEndpoint is used here purely as a validation gate.
	if _, err := NormalizeEndpoint(endpoint); err != nil {
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
			return Metadata{}, &EdgeGatewayUnsupportedError{Endpoint: endpoint, Cause: in.DiscoveryErr}
		}
		issuer = in.Discovered.Issuer
		if audience == "" {
			audience = in.Discovered.Audience
		}
	}

	clientID := in.OIDCClientID
	if clientID == "" {
		// Always write an explicit oidc_client_id (defaulting the same way
		// Metadata.OIDCClientIDOrDefault would at read time) so the
		// registration is self-describing rather than relying on every
		// reader to know the default.
		clientID = Metadata{}.OIDCClientIDOrDefault()
	}

	m := Metadata{
		Name:            name,
		GatewayEndpoint: endpoint,
		IsRemote:        true,
		GatewayPort:     0,
		AuthMode:        AuthModeOIDC,
		OIDCIssuer:      &issuer,
		OIDCClientID:    &clientID,
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

// EnsureScheme defaults a missing scheme to https:// and validates the
// result parses as a URL with a non-empty host. Exported so internal/cli can
// normalize an endpoint before an OIDC-discovery probe, using the exact same
// rule NewMetadata applies when it normalizes again internally.
//
// Named EnsureScheme (not NormalizeEndpoint) to leave that name free for
// Feature C's endpoint-matching normalization (default-port stripping,
// lower-casing) — a different operation that happens to want the same
// obvious name.
func EnsureScheme(endpoint string) (string, error) {
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

// existsInUserTree reports whether name has a metadata.json in the user tree
// specifically (not the system tree) — the write-side existence check.
// Unlike Exists/Load (which walk user-then-system for reads, since the user
// entry is documented to shadow the system entry), a write must only be
// blocked by a conflicting entry it would actually collide with: the user
// tree is the only tree OSWriter ever writes to, so a name that exists only
// in the system tree is a legitimate shadow registration, not a conflict.
func existsInUserTree(env Env, name string) (bool, error) {
	if err := ValidateGatewayName(name); err != nil {
		return false, err
	}
	if env.UserFS == nil {
		return false, nil
	}
	_, present, err := readMetadataFile(env.UserFS, name)
	if err != nil {
		return false, err
	}
	return present, nil
}

// Exists reports whether a gateway named name is already registered (user or
// system tree). Unlike Load, a "not found" result is not an error — any other
// error (an invalid name, or a present-but-unparseable metadata.json) is
// propagated.
func Exists(env Env, name string) (bool, error) {
	_, err := Load(env, name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrGatewayNotFound) {
		return false, nil
	}
	return false, err
}

// WriteGateway writes m as gateways/<m.Name>/metadata.json via w (0600, 0700
// parent dir, atomic temp+rename — all already provided by the real Writer
// implementation, OSWriter; WriteGateway itself adds no new I/O primitive, it
// just drives the existing one correctly).
//
// Fails with GatewayExistsError if m.Name is already registered **in the
// user tree** — a name that exists only in the system tree is a legitimate
// shadow registration, not a conflict (see existsInUserTree). Pass force to
// skip this check entirely and overwrite — the explicit re-registration path
// (gateway add --force), as opposed to the default refusal to clobber a name
// by accident.
func WriteGateway(w Writer, env Env, m Metadata, force bool) error {
	if err := ValidateGatewayName(m.Name); err != nil {
		return err
	}
	if !force {
		exists, err := existsInUserTree(env, m.Name)
		if err != nil {
			return err
		}
		if exists {
			return &GatewayExistsError{Name: m.Name}
		}
	}
	data, err := m.Marshal()
	if err != nil {
		return err
	}
	return w.WriteFile(GatewayDir(m.Name)+"/metadata.json", data, 0o600)
}
