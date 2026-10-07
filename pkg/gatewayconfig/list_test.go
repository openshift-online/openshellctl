package gatewayconfig

import (
	"testing"
	"testing/fstest"
)

func TestTypeLabel(t *testing.T) {
	if got := TypeLabel(true); got != "remote" {
		t.Errorf("TypeLabel(true) = %q, want remote", got)
	}
	if got := TypeLabel(false); got != "local" {
		t.Errorf("TypeLabel(false) = %q, want local", got)
	}
}

func TestAuthLabel(t *testing.T) {
	tests := []struct {
		mode AuthMode
		want string
	}{
		{AuthModeOIDC, "oidc"},
		{AuthModeMTLS, "mtls"},
		{AuthModeCloudflareJWT, "cloudflare_jwt"},
		{AuthModeNone, "none"},
		{AuthModePlaintext, "none"},
		{AuthModeUnset, "unknown"},
		{AuthMode("something-new"), "unknown"},
	}
	for _, tt := range tests {
		if got := AuthLabel(tt.mode); got != tt.want {
			t.Errorf("AuthLabel(%q) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestListDetailed_Empty(t *testing.T) {
	env := Env{UserFS: fstest.MapFS{}}
	infos, err := ListDetailed(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 0 {
		t.Errorf("got %d entries, want 0", len(infos))
	}
}

func TestListDetailed_PopulatesFields(t *testing.T) {
	issuerJSON := `{"name":"rosa","gateway_endpoint":"https://gw.example.com","is_remote":true,"gateway_port":0,"auth_mode":"oidc"}`
	env := Env{
		UserFS: mapFSWith(map[string]string{
			"gateways/rosa/metadata.json": issuerJSON,
		}),
	}
	infos, err := ListDetailed(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatalf("got %d entries, want 1", len(infos))
	}
	got := infos[0]
	if got.Name != "rosa" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Endpoint != "https://gw.example.com" {
		t.Errorf("Endpoint = %q", got.Endpoint)
	}
	if got.Type != "remote" {
		t.Errorf("Type = %q, want remote", got.Type)
	}
	if got.Auth != "oidc" {
		t.Errorf("Auth = %q, want oidc", got.Auth)
	}
}

func TestListDetailed_MarksActive(t *testing.T) {
	env := Env{
		UserFS: mapFSWith(map[string]string{
			"gateways/a/metadata.json": md("a", "https://a"),
			"gateways/b/metadata.json": md("b", "https://b"),
			"active_gateway":           "b",
		}),
	}
	infos, err := ListDetailed(env)
	if err != nil {
		t.Fatal(err)
	}
	active := map[string]bool{}
	for _, i := range infos {
		active[i.Name] = i.Active
	}
	if active["b"] != true || active["a"] != false {
		t.Errorf("active map = %+v, want only b active", active)
	}
}
