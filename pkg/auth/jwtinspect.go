package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrNotJWT is returned by Inspect when the input is not a JWT with three
// base64url segments whose payload is a JSON object.
var ErrNotJWT = errors.New("not a JWT")

// Claims are the decoded, UNVERIFIED contents of a JWT payload. Decoding never
// checks the signature (no key material is available client-side), so Claims
// must never be trusted for authorization decisions — only for display and for
// populating a Token's metadata (spec §5.2).
type Claims struct {
	Iss string
	Sub string
	Aud []string
	Exp int64
	Iat int64
	Nbf int64

	PreferredUsername string
	Roles             []string // realm_access.roles
	Scope             string

	Raw map[string]any
}

// Inspect decodes the payload segment of a JWT without verifying its signature.
// aud may be encoded as a string or an array of strings; both are normalised to
// a []string. Returns ErrNotJWT when the input is not three base64url segments
// or its payload is not a JSON object.
func Inspect(jwt string) (*Claims, error) {
	segments := strings.Split(jwt, ".")
	if len(segments) != 3 {
		return nil, fmt.Errorf("%w: expected 3 segments, got %d", ErrNotJWT, len(segments))
	}

	payload, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return nil, fmt.Errorf("%w: payload is not base64url: %v", ErrNotJWT, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("%w: payload is not a JSON object: %v", ErrNotJWT, err)
	}

	c := &Claims{Raw: raw}
	c.Iss = stringClaim(raw, "iss")
	c.Sub = stringClaim(raw, "sub")
	c.Aud = audienceClaim(raw["aud"])
	c.Exp = intClaim(raw, "exp")
	c.Iat = intClaim(raw, "iat")
	c.Nbf = intClaim(raw, "nbf")
	c.PreferredUsername = stringClaim(raw, "preferred_username")
	c.Scope = stringClaim(raw, "scope")
	c.Roles = realmAccessRoles(raw)

	return c, nil
}

func stringClaim(raw map[string]any, key string) string {
	if v, ok := raw[key].(string); ok {
		return v
	}
	return ""
}

// intClaim reads a numeric claim. JSON numbers decode to float64.
func intClaim(raw map[string]any, key string) int64 {
	switch v := raw[key].(type) {
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	default:
		return 0
	}
}

// audienceClaim normalises the aud claim, which may be a string or []string.
func audienceClaim(v any) []string {
	switch aud := v.(type) {
	case string:
		if aud == "" {
			return nil
		}
		return []string{aud}
	case []any:
		out := make([]string, 0, len(aud))
		for _, item := range aud {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// realmAccessRoles extracts realm_access.roles (Keycloak layout).
func realmAccessRoles(raw map[string]any) []string {
	ra, ok := raw["realm_access"].(map[string]any)
	if !ok {
		return nil
	}
	rolesAny, ok := ra["roles"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(rolesAny))
	for _, r := range rolesAny {
		if s, ok := r.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
