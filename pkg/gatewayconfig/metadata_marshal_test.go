package gatewayconfig

import "testing"

// TestMetadata_MarshalGolden pins Marshal's output against the real CronJob
// heredoc in openshift-online/rosa-agent (sandbox/skills/job-sop-improve/
// job-sop-improve-cron.yaml, commit a9ba80b7e9694bd47ccf8c88710ab1b923f9174c;
// confirmed byte-identical in job-ops-sop-pr-review's CronJob too) — not a
// guessed fixture.
func TestMetadata_MarshalGolden(t *testing.T) {
	issuer := "https://issuer.example.com"
	clientID := "client-abc"
	audience := "aud-abc"
	m := Metadata{
		Name:            "my-gw",
		GatewayEndpoint: "https://gw.example.com:443",
		IsRemote:        true,
		GatewayPort:     0,
		AuthMode:        AuthModeOIDC,
		OIDCIssuer:      &issuer,
		OIDCClientID:    &clientID,
		OIDCAudience:    &audience,
	}
	got, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "name": "my-gw",
  "gateway_endpoint": "https://gw.example.com:443",
  "is_remote": true,
  "gateway_port": 0,
  "auth_mode": "oidc",
  "oidc_issuer": "https://issuer.example.com",
  "oidc_client_id": "client-abc",
  "oidc_audience": "aud-abc"
}`
	if string(got) != want {
		t.Errorf("Marshal mismatch:\n got: %q\nwant: %q", string(got), want)
	}
}

// TestMetadata_MarshalOmitsNilOptionals pins the minimal shape against the
// in-repo upstream-written sample already used by config_test.go's md()
// helper: {"name":...,"gateway_endpoint":...,"is_remote":false,"gateway_port":N}
// with every optional field (including auth_mode) absent.
func TestMetadata_MarshalOmitsNilOptionals(t *testing.T) {
	m := Metadata{
		Name:            "g",
		GatewayEndpoint: "https://x",
		IsRemote:        false,
		GatewayPort:     8080,
	}
	got, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "name": "g",
  "gateway_endpoint": "https://x",
  "is_remote": false,
  "gateway_port": 8080
}`
	if string(got) != want {
		t.Errorf("Marshal mismatch:\n got: %q\nwant: %q", string(got), want)
	}
}

// TestMetadata_MarshalRoundTrip confirms Marshal's output parses back via
// ParseMetadata to the same Metadata (field-for-field).
func TestMetadata_MarshalRoundTrip(t *testing.T) {
	issuer := "https://issuer"
	m := Metadata{
		Name:            "rt",
		GatewayEndpoint: "https://gw",
		IsRemote:        true,
		GatewayPort:     1,
		AuthMode:        AuthModeOIDC,
		OIDCIssuer:      &issuer,
	}
	data, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseMetadata("rt", data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != m.Name || got.GatewayEndpoint != m.GatewayEndpoint ||
		got.IsRemote != m.IsRemote || got.GatewayPort != m.GatewayPort ||
		got.AuthMode != m.AuthMode || got.OIDCIssuer == nil || *got.OIDCIssuer != *m.OIDCIssuer {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, m)
	}
}
