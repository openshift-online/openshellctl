package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func makeTestJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return enc(map[string]any{"alg": "RS256"}) + "." + enc(payload) + ".c2ln"
}

func TestTokenInspect_Text(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	jwt := makeTestJWT(t, map[string]any{
		"iss":          "https://issuer",
		"sub":          "user-42",
		"aud":          []any{"openshell-cli"},
		"realm_access": map[string]any{"roles": []any{"openshell-user"}},
	})

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"token", "inspect", jwt})

	if err := root.Execute(); err != nil {
		t.Fatalf("token inspect: %v", err)
	}
	got := out.String()
	for _, want := range []string{"user-42", "openshell-cli", "openshell-user", "https://issuer"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got:\n%s", want, got)
		}
	}
}

func TestTokenInspect_JSON(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	jwt := makeTestJWT(t, map[string]any{"iss": "https://i", "sub": "s"})
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"token", "inspect", jwt, "-o", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("token inspect -o json: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out.String())
	}
	if parsed["sub"] != "s" {
		t.Errorf("sub = %v, want s", parsed["sub"])
	}
}

func TestTokenInspect_NotJWTExitsError(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"token", "inspect", "not-a-jwt"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for a non-JWT")
	}
}
