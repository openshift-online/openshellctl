package gatewayconfig

import (
	"errors"
	"testing"
	"testing/fstest"
)

func md(name, endpoint string) string {
	return `{"name":"` + name + `","gateway_endpoint":"` + endpoint + `","is_remote":false,"gateway_port":8080}`
}

func TestLoad_UserShadowsSystem(t *testing.T) {
	env := Env{
		Getenv: func(string) string { return "" },
		UserFS: fstest.MapFS{
			"gateways/rosa/metadata.json": {Data: []byte(md("rosa", "https://user"))},
		},
		SysFS: fstest.MapFS{
			"gateways/rosa/metadata.json": {Data: []byte(md("rosa", "https://system"))},
		},
	}
	r, err := Load(env, "rosa")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if r.Source != SourceUser {
		t.Errorf("Source = %v, want user", r.Source)
	}
	if r.Metadata.GatewayEndpoint != "https://user" {
		t.Errorf("endpoint = %q, want user", r.Metadata.GatewayEndpoint)
	}
}

func TestLoad_FallsBackToSystem(t *testing.T) {
	env := Env{
		Getenv: func(string) string { return "" },
		UserFS: fstest.MapFS{},
		SysFS: fstest.MapFS{
			"gateways/rosa/metadata.json": {Data: []byte(md("rosa", "https://system"))},
		},
	}
	r, err := Load(env, "rosa")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if r.Source != SourceSystem {
		t.Errorf("Source = %v, want system", r.Source)
	}
}

func TestLoad_InvalidUserShadowsValidSystem(t *testing.T) {
	env := Env{
		Getenv: func(string) string { return "" },
		UserFS: fstest.MapFS{
			"gateways/rosa/metadata.json": {Data: []byte(`{not json`)},
		},
		SysFS: fstest.MapFS{
			"gateways/rosa/metadata.json": {Data: []byte(md("rosa", "https://system"))},
		},
	}
	_, err := Load(env, "rosa")
	if !errors.Is(err, ErrMetadataParse) {
		t.Fatalf("err = %v, want ErrMetadataParse (user shadows system even when invalid)", err)
	}
}

func TestLoad_NotFound(t *testing.T) {
	env := Env{Getenv: func(string) string { return "" }, UserFS: fstest.MapFS{}, SysFS: fstest.MapFS{}}
	_, err := Load(env, "nope")
	if !errors.Is(err, ErrGatewayNotFound) {
		t.Fatalf("err = %v, want ErrGatewayNotFound", err)
	}
}

func TestLoad_InvalidName(t *testing.T) {
	env := Env{Getenv: func(string) string { return "" }}
	_, err := Load(env, "a/b")
	if !errors.Is(err, ErrInvalidGatewayName) {
		t.Fatalf("err = %v, want ErrInvalidGatewayName", err)
	}
}

func TestActiveGateway(t *testing.T) {
	env := Env{
		UserFS: fstest.MapFS{"active_gateway": {Data: []byte("  rosa\n")}},
	}
	got, err := ActiveGateway(env)
	if err != nil {
		t.Fatalf("ActiveGateway: %v", err)
	}
	if got != "rosa" {
		t.Errorf("active = %q, want rosa (trimmed)", got)
	}
}

func TestActiveGateway_UserThenSystem(t *testing.T) {
	env := Env{
		UserFS: fstest.MapFS{},
		SysFS:  fstest.MapFS{"active_gateway": {Data: []byte("sys-gw")}},
	}
	got, err := ActiveGateway(env)
	if err != nil {
		t.Fatalf("ActiveGateway: %v", err)
	}
	if got != "sys-gw" {
		t.Errorf("active = %q, want sys-gw", got)
	}
}

func TestActiveGateway_EmptyAndInvalidIgnored(t *testing.T) {
	env := Env{UserFS: fstest.MapFS{"active_gateway": {Data: []byte("   \n")}}}
	if _, err := ActiveGateway(env); !errors.Is(err, ErrNoActiveGateway) {
		t.Errorf("empty active: err = %v, want ErrNoActiveGateway", err)
	}

	env2 := Env{UserFS: fstest.MapFS{"active_gateway": {Data: []byte("a/b")}}}
	if _, err := ActiveGateway(env2); !errors.Is(err, ErrNoActiveGateway) {
		t.Errorf("invalid active name: err = %v, want ErrNoActiveGateway", err)
	}
}

func TestFindByEndpoint_TrailingSlash(t *testing.T) {
	env := Env{
		UserFS: fstest.MapFS{
			"gateways/rosa/metadata.json": {Data: []byte(md("rosa", "https://gw.example.com"))},
			"active_gateway":              {Data: []byte("rosa")},
		},
	}
	name, ok, err := FindByEndpoint(env, "https://gw.example.com///")
	if err != nil || !ok {
		t.Fatalf("FindByEndpoint ok=%v err=%v", ok, err)
	}
	if name != "rosa" {
		t.Errorf("name = %q, want rosa", name)
	}
}

func TestResolve_EndpointOnly(t *testing.T) {
	env := Env{UserFS: fstest.MapFS{}, SysFS: fstest.MapFS{}}
	tgt, err := Resolve(env, ResolveInput{Endpoint: "https://direct:8080"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if tgt.Endpoint != "https://direct:8080" {
		t.Errorf("endpoint = %q", tgt.Endpoint)
	}
	if tgt.Name != "https://direct:8080" {
		t.Errorf("name = %q, want the raw endpoint fallback", tgt.Name)
	}
	if tgt.Resolved != nil {
		t.Errorf("Resolved should be nil for endpoint-only")
	}
}

func TestResolve_EndpointMatchesMetadata(t *testing.T) {
	env := Env{
		UserFS: fstest.MapFS{
			"gateways/rosa/metadata.json": {Data: []byte(md("rosa", "https://gw"))},
			"active_gateway":              {Data: []byte("rosa")},
		},
	}
	tgt, err := Resolve(env, ResolveInput{Endpoint: "https://gw"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if tgt.Name != "rosa" {
		t.Errorf("name = %q, want rosa", tgt.Name)
	}
	if tgt.Resolved == nil {
		t.Errorf("Resolved should be populated when endpoint matches a gateway")
	}
	if tgt.Endpoint != "https://gw" {
		t.Errorf("endpoint = %q, want the provided endpoint", tgt.Endpoint)
	}
}

func TestResolve_NameThenActive(t *testing.T) {
	env := Env{
		UserFS: fstest.MapFS{
			"gateways/rosa/metadata.json": {Data: []byte(md("rosa", "https://gw"))},
			"active_gateway":              {Data: []byte("rosa")},
		},
	}
	tgt, err := Resolve(env, ResolveInput{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if tgt.Name != "rosa" || tgt.Endpoint != "https://gw" {
		t.Errorf("got name=%q endpoint=%q", tgt.Name, tgt.Endpoint)
	}
}

func TestResolve_NoActive(t *testing.T) {
	env := Env{UserFS: fstest.MapFS{}, SysFS: fstest.MapFS{}}
	_, err := Resolve(env, ResolveInput{})
	if !errors.Is(err, ErrNoActiveGateway) {
		t.Fatalf("err = %v, want ErrNoActiveGateway", err)
	}
}

func TestResolve_UnknownName(t *testing.T) {
	env := Env{UserFS: fstest.MapFS{}, SysFS: fstest.MapFS{}}
	_, err := Resolve(env, ResolveInput{Name: "ghost"})
	if !errors.Is(err, ErrUnknownGateway) {
		t.Fatalf("err = %v, want ErrUnknownGateway", err)
	}
}

func TestMetadata_CFAliases(t *testing.T) {
	raw := []byte(`{"name":"g","gateway_endpoint":"https://x","is_remote":false,"gateway_port":1,"cf_team_domain":"team.example","cf_auth_url":"https://auth"}`)
	m, err := ParseMetadata("g", raw)
	if err != nil {
		t.Fatalf("ParseMetadata: %v", err)
	}
	if m.EdgeTeamDomain == nil || *m.EdgeTeamDomain != "team.example" {
		t.Errorf("EdgeTeamDomain not read from cf_team_domain alias: %v", m.EdgeTeamDomain)
	}
	if m.EdgeAuthURL == nil || *m.EdgeAuthURL != "https://auth" {
		t.Errorf("EdgeAuthURL not read from cf_auth_url alias: %v", m.EdgeAuthURL)
	}
}

func TestMetadata_OIDCClientIDDefault(t *testing.T) {
	m := Metadata{}
	if got := m.OIDCClientIDOrDefault(); got != "openshell-cli" {
		t.Errorf("default client id = %q, want openshell-cli", got)
	}
	cid := "custom"
	m.OIDCClientID = &cid
	if got := m.OIDCClientIDOrDefault(); got != "custom" {
		t.Errorf("client id = %q, want custom", got)
	}
}

func TestList(t *testing.T) {
	env := Env{
		UserFS: fstest.MapFS{
			"gateways/rosa/metadata.json": {Data: []byte(md("rosa", "https://a"))},
			"active_gateway":              {Data: []byte("rosa")},
		},
		SysFS: fstest.MapFS{
			"gateways/rosa/metadata.json": {Data: []byte(md("rosa", "https://sys"))},
			"gateways/prod/metadata.json": {Data: []byte(md("prod", "https://p"))},
		},
	}
	infos, err := List(env)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byName := map[string]Info{}
	for _, i := range infos {
		byName[i.Name] = i
	}
	if len(byName) != 2 {
		t.Fatalf("got %d gateways, want 2: %+v", len(byName), infos)
	}
	if byName["rosa"].Source != SourceUser || !byName["rosa"].Active {
		t.Errorf("rosa = %+v, want user+active", byName["rosa"])
	}
	if byName["prod"].Source != SourceSystem || byName["prod"].Active {
		t.Errorf("prod = %+v, want system+inactive", byName["prod"])
	}
}
