package vaultconfig

import "strings"

// fieldSuffixes are the auth-relevant openshellctl config fields Vault is
// allowed to fill, in a fixed order for deterministic iteration. The static
// bearer token (--token/OPENSHELL_TOKEN) is deliberately excluded: auth.Resolve
// checks it before the client-credentials path, so a Vault-sourced token would
// silently override an explicitly-configured client secret, and tokens are
// short-lived JWTs that don't belong cached in Vault.
var fieldSuffixes = []string{
	"gateway",
	"gateway-endpoint",
	"oidc-issuer",
	"oidc-client-id",
	"oidc-client-secret",
	"oidc-audience",
	"oidc-scopes",
}

// NormalizePrefix strips all trailing "-" characters from prefix, so
// "hypershell-" and "hypershell" build identical field names (SecretFieldName
// always joins with exactly one "-").
func NormalizePrefix(prefix string) string {
	return strings.TrimRight(prefix, "-")
}

// SecretFieldName returns the Vault secret field name for a given field
// suffix under prefix (e.g. SecretFieldName("hypershell", "oidc-client-id")
// -> "hypershell-oidc-client-id"; SecretFieldName("", "gateway") -> "gateway").
func SecretFieldName(prefix, suffix string) string {
	prefix = NormalizePrefix(prefix)
	if prefix == "" {
		return suffix
	}
	return prefix + "-" + suffix
}

// ViperKey maps a field suffix to the viper key it should be written to. Every
// suffix maps to itself except "oidc-client-secret", which maps to the dotted
// "oidc.client_secret" — matching the existing asymmetry in
// internal/cli/authwiring.go's clientSecretProvider, which reads that key
// directly rather than through a bound flag.
func ViperKey(suffix string) string {
	if suffix == "oidc-client-secret" {
		return "oidc.client_secret"
	}
	return suffix
}

// Overrides records which of the paired auth fields are already set by an
// explicit flag or OPENSHELL_* env var, as seen by the caller before any
// Vault defaults are applied. ToViperDefaults uses this to keep each pair
// atomic (see the package doc and docs/plans/0003 §3.1) rather than letting
// Vault silently fill one half of a pair while the other half comes from
// somewhere else.
type Overrides struct {
	// Gateway is true when --gateway/OPENSHELL_GATEWAY is set.
	Gateway bool
	// GatewayEndpoint is true when --gateway-endpoint/OPENSHELL_GATEWAY_ENDPOINT is set.
	GatewayEndpoint bool
	// OIDCClientID is true when --oidc-client-id/OPENSHELL_OIDC_CLIENT_ID is set.
	OIDCClientID bool
	// ClientSecretSource is true when OPENSHELL_OIDC_CLIENT_SECRET or
	// --client-secret-file is set.
	ClientSecretSource bool
}

// ExtractFields reads the known field suffixes out of a raw Vault secret's
// data (as returned by the KV v2 API), applying prefix. A suffix absent from
// data is skipped, not an error — most Vault secrets only carry a subset of
// the fields. A key in data that doesn't match any known (prefixed) suffix is
// ignored. A recognized field whose value is not a string is an error.
func ExtractFields(data map[string]any, prefix string) (map[string]string, error) {
	out := make(map[string]string, len(fieldSuffixes))
	for _, suffix := range fieldSuffixes {
		key := SecretFieldName(prefix, suffix)
		raw, ok := data[key]
		if !ok {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			return nil, &ErrFieldNotString{Field: key}
		}
		out[suffix] = s
	}
	return out, nil
}

// ToViperDefaults converts the extracted field suffix -> value map into the
// final viper key -> value map ready for viper.SetDefault, applying:
//   - precedence: a field with an empty value is skipped (never defaults an
//     "unset" key to another kind of "unset");
//   - pairing: if either half of the gateway/gateway-endpoint pair, or either
//     half of the oidc-client-id/oidc-client-secret pair, is already
//     overridden (per ov), both halves of that pair are excluded from the
//     result, even if Vault supplied a value for them.
func ToViperDefaults(fields map[string]string, ov Overrides) map[string]string {
	gatewayPairOverridden := ov.Gateway || ov.GatewayEndpoint
	clientPairOverridden := ov.OIDCClientID || ov.ClientSecretSource

	out := make(map[string]string, len(fields))
	for suffix, val := range fields {
		if val == "" {
			continue
		}
		switch suffix {
		case "gateway", "gateway-endpoint":
			if gatewayPairOverridden {
				continue
			}
		case "oidc-client-id", "oidc-client-secret":
			if clientPairOverridden {
				continue
			}
		}
		out[ViperKey(suffix)] = val
	}
	return out
}
