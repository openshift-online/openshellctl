package gatewayconfig

import (
	"encoding/json"
	"io/fs"
)

// AuthMode is the metadata.json auth_mode value.
type AuthMode string

// Auth modes recognised in metadata.json's auth_mode field. AuthModeUnset (the
// absent value) is treated by upstream as mTLS for https endpoints and plaintext
// for http:// (commands/gateway.rs:581-587).
const (
	AuthModeUnset         AuthMode = ""
	AuthModeNone          AuthMode = "none"
	AuthModePlaintext     AuthMode = "plaintext"
	AuthModeCloudflareJWT AuthMode = "cloudflare_jwt"
	AuthModeOIDC          AuthMode = "oidc"
	AuthModeMTLS          AuthMode = "mtls"
)

// Metadata mirrors GatewayMetadata (metadata.rs:14-74). Unknown fields are
// ignored on read. The four leading fields are always emitted; Option fields
// use omitempty. openshellctl never writes metadata.json in v1 (read-only), but
// the tags are kept faithful for round-trips.
type Metadata struct {
	Name             string   `json:"name"`
	GatewayEndpoint  string   `json:"gateway_endpoint"`
	IsRemote         bool     `json:"is_remote"`
	GatewayPort      uint16   `json:"gateway_port"`
	RemoteHost       *string  `json:"remote_host,omitempty"`
	ResolvedHost     *string  `json:"resolved_host,omitempty"`
	AuthMode         AuthMode `json:"auth_mode,omitempty"`
	EdgeTeamDomain   *string  `json:"edge_team_domain,omitempty"`
	EdgeAuthURL      *string  `json:"edge_auth_url,omitempty"`
	OIDCIssuer       *string  `json:"oidc_issuer,omitempty"`
	OIDCClientID     *string  `json:"oidc_client_id,omitempty"`
	OIDCAudience     *string  `json:"oidc_audience,omitempty"`
	OIDCScopes       *string  `json:"oidc_scopes,omitempty"`
	VMDriverStateDir *string  `json:"vm_driver_state_dir,omitempty"`
}

// metadataAliases captures the serde read-aliases upstream accepts
// (cf_team_domain -> edge_team_domain, cf_auth_url -> edge_auth_url). Go's
// encoding/json has no alias support, so we decode a second time into this shim
// and fill any unset primary fields.
type metadataAliases struct {
	CFTeamDomain *string `json:"cf_team_domain,omitempty"`
	CFAuthURL    *string `json:"cf_auth_url,omitempty"`
}

// ParseMetadata decodes metadata.json bytes, honoring the cf_* read-aliases.
func ParseMetadata(name string, b []byte) (Metadata, error) {
	var m Metadata
	if err := json.Unmarshal(b, &m); err != nil {
		return Metadata{}, &MetadataParseError{Name: name, Cause: err}
	}
	var alias metadataAliases
	if err := json.Unmarshal(b, &alias); err == nil {
		if m.EdgeTeamDomain == nil && alias.CFTeamDomain != nil {
			m.EdgeTeamDomain = alias.CFTeamDomain
		}
		if m.EdgeAuthURL == nil && alias.CFAuthURL != nil {
			m.EdgeAuthURL = alias.CFAuthURL
		}
	}
	return m, nil
}

// OIDCClientIDOrDefault returns the configured oidc_client_id or the upstream
// default "openshell-cli" (applied at login time, commands/gateway.rs:1137).
func (m Metadata) OIDCClientIDOrDefault() string {
	if m.OIDCClientID != nil && *m.OIDCClientID != "" {
		return *m.OIDCClientID
	}
	return "openshell-cli"
}

// readMetadataFile reads and parses gateways/<name>/metadata.json from fsys.
func readMetadataFile(fsys fs.FS, name string) (Metadata, bool, error) {
	rel := GatewayDir(name) + "/metadata.json"
	b, err := fs.ReadFile(fsys, rel)
	if err != nil {
		return Metadata{}, false, nil // not found (or unreadable) → treat as absent
	}
	m, perr := ParseMetadata(name, b)
	if perr != nil {
		return Metadata{}, true, perr // present but invalid
	}
	return m, true, nil
}
