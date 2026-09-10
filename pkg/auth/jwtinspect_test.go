package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
)

// makeJWT builds an unsigned-looking JWT (header.payload.signature) from a
// payload map. The signature segment is arbitrary; Inspect never verifies it.
func makeJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	header := b64(t, map[string]any{"alg": "RS256", "typ": "JWT"})
	body := b64(t, payload)
	return header + "." + body + ".c2ln" // "sig"
}

func b64(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func TestInspect_AudienceAsString(t *testing.T) {
	jwt := makeJWT(t, map[string]any{
		"iss": "https://issuer.example.com",
		"sub": "user-1",
		"aud": "openshell-cli",
		"exp": float64(2000000000),
		"iat": float64(1000000000),
	})
	c, err := Inspect(jwt)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(c.Aud) != 1 || c.Aud[0] != "openshell-cli" {
		t.Errorf("Aud = %v, want [openshell-cli]", c.Aud)
	}
	if c.Iss != "https://issuer.example.com" {
		t.Errorf("Iss = %q", c.Iss)
	}
	if c.Sub != "user-1" {
		t.Errorf("Sub = %q", c.Sub)
	}
	if c.Exp != 2000000000 {
		t.Errorf("Exp = %d", c.Exp)
	}
	if c.Iat != 1000000000 {
		t.Errorf("Iat = %d", c.Iat)
	}
}

func TestInspect_AudienceAsArray(t *testing.T) {
	jwt := makeJWT(t, map[string]any{
		"iss": "https://i",
		"aud": []any{"openshell-cli", "account"},
	})
	c, err := Inspect(jwt)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(c.Aud) != 2 || c.Aud[0] != "openshell-cli" || c.Aud[1] != "account" {
		t.Errorf("Aud = %v, want [openshell-cli account]", c.Aud)
	}
}

func TestInspect_RealmAccessRoles(t *testing.T) {
	jwt := makeJWT(t, map[string]any{
		"iss": "https://i",
		"realm_access": map[string]any{
			"roles": []any{"openshell-user", "default-roles"},
		},
		"preferred_username": "alice",
		"scope":              "openid profile",
	})
	c, err := Inspect(jwt)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(c.Roles) != 2 || c.Roles[0] != "openshell-user" {
		t.Errorf("Roles = %v", c.Roles)
	}
	if c.PreferredUsername != "alice" {
		t.Errorf("PreferredUsername = %q", c.PreferredUsername)
	}
	if c.Scope != "openid profile" {
		t.Errorf("Scope = %q", c.Scope)
	}
}

func TestInspect_MissingRealmAccess(t *testing.T) {
	jwt := makeJWT(t, map[string]any{"iss": "https://i", "sub": "s"})
	c, err := Inspect(jwt)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(c.Roles) != 0 {
		t.Errorf("Roles = %v, want empty", c.Roles)
	}
}

func TestInspect_NotJWT(t *testing.T) {
	cases := []string{
		"",
		"not-a-jwt",
		"only.two",
		"a.b.c.d",
	}
	for _, in := range cases {
		if _, err := Inspect(in); !errors.Is(err, ErrNotJWT) {
			t.Errorf("Inspect(%q) err = %v, want ErrNotJWT", in, err)
		}
	}
}

func TestInspect_PayloadNotJSONObject(t *testing.T) {
	// A valid 3-segment token whose payload is a JSON array, not an object.
	jwt := "aGVhZGVy." + base64.RawURLEncoding.EncodeToString([]byte(`[1,2,3]`)) + ".c2ln"
	if _, err := Inspect(jwt); !errors.Is(err, ErrNotJWT) {
		t.Errorf("Inspect(array payload) err = %v, want ErrNotJWT", err)
	}
}

func TestInspect_RawPopulated(t *testing.T) {
	jwt := makeJWT(t, map[string]any{"iss": "https://i", "custom": "x"})
	c, err := Inspect(jwt)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if c.Raw["custom"] != "x" {
		t.Errorf("Raw[custom] = %v, want x", c.Raw["custom"])
	}
}
