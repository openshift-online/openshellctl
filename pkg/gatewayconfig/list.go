package gatewayconfig

// DetailedInfo is a gateway list entry with metadata resolved — Info (Name,
// Source, Active) plus endpoint and human-readable type/auth labels.
type DetailedInfo struct {
	Info
	Endpoint string
	Type     string // TypeLabel(Metadata.IsRemote)
	Auth     string // AuthLabel(Metadata.AuthMode)
}

// TypeLabel renders Metadata.IsRemote as "remote" or "local".
func TypeLabel(isRemote bool) string {
	if isRemote {
		return "remote"
	}
	return "local"
}

// AuthLabel renders an AuthMode as the label `gateway list` displays.
// AuthModeNone and AuthModePlaintext both mean "no authentication" from a
// user's perspective and share the "none" label; AuthModeUnset (metadata.json
// has no auth_mode field) and any unrecognized value render as "unknown"
// rather than guessing — openshellctl's own mTLS-vs-plaintext inference for
// AuthModeUnset (see metadata.go's doc comment) is a read-time concern for
// dialing, not something `gateway list` asserts about a registration.
func AuthLabel(mode AuthMode) string {
	switch mode {
	case AuthModeOIDC:
		return "oidc"
	case AuthModeMTLS:
		return "mtls"
	case AuthModeCloudflareJWT:
		return "cloudflare_jwt"
	case AuthModeNone, AuthModePlaintext:
		return "none"
	default:
		return "unknown"
	}
}

// ListDetailed is List (config.go) with each entry's metadata resolved into
// endpoint/type/auth labels, for `gateway list`'s table/json/yaml output. A
// gateway whose metadata.json fails to load is skipped (List already only
// returns entries with present, structurally-valid metadata — see
// readMetadataFile's "present" bool).
func ListDetailed(env Env) ([]DetailedInfo, error) {
	infos, err := List(env)
	if err != nil {
		return nil, err
	}
	out := make([]DetailedInfo, 0, len(infos))
	for _, info := range infos {
		r, err := Load(env, info.Name)
		if err != nil {
			continue
		}
		out = append(out, DetailedInfo{
			Info:     info,
			Endpoint: r.Metadata.GatewayEndpoint,
			Type:     TypeLabel(r.Metadata.IsRemote),
			Auth:     AuthLabel(r.Metadata.AuthMode),
		})
	}
	return out, nil
}
