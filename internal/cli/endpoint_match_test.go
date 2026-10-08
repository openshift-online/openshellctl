package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTokenShow_GatewayEndpointWithTrailingSlashMatchesRegisteredDefaultPort
// is this story's own acceptance criterion: an env/flag endpoint with a
// trailing slash and no port must still resolve to a gateway registered with
// an explicit default port (:443) — the literal onboarding-thread bug
// (OPENSHELL_GATEWAY_ENDPOINT exported with a trailing slash while `gateway
// add`/the CronJobs register the same host as https://host:443). Before
// this story, FindByEndpoint's bare trailing-slash trim did not match these,
// and the registered gateway's auth mode, token file, and TLS material were
// silently ignored; `token show` would fall through to an endpoint-only,
// unauthenticated resolution instead of using the registered oidc disk
// bundle.
func TestTokenShow_GatewayEndpointWithTrailingSlashMatchesRegisteredDefaultPort(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "config")
	gwDir := filepath.Join(xdg, "openshell", "gateways", "rosa")
	if err := os.MkdirAll(gwDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Registered the way `gateway add`/the CronJobs do: an explicit default
	// port.
	md := `{"name":"rosa","gateway_endpoint":"https://gw.example.com:443","is_remote":true,"gateway_port":0,"auth_mode":"oidc","oidc_issuer":"https://issuer"}`
	if err := os.WriteFile(filepath.Join(gwDir, "metadata.json"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle := `{"access_token":"tok","expires_at":9999999999,"issuer":"https://issuer","client_id":"openshell-cli"}`
	if err := os.WriteFile(filepath.Join(gwDir, "oidc_token.json"), []byte(bundle), 0o600); err != nil {
		t.Fatal(err)
	}
	// Deliberately no active_gateway file: this must be found purely by
	// FindByEndpoint's endpoint match, not an active-gateway shortcut.
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	// Exported the way a service account would: trailing slash, no port.
	out, err := runCmd(t, "token", "show", "--gateway-endpoint", "https://gw.example.com/")
	if err != nil {
		t.Fatalf("token show: %v", err)
	}
	if !strings.Contains(out, "gateway=rosa") {
		t.Errorf("expected a gateway-backed source (oidc_token.json (gateway=rosa)), got:\n%s", out)
	}
}

// TestTokenShow_GatewayEndpointEnvVar_TrailingSlashMatchesDefaultPort is the
// same scenario driven through the actual environment variable
// (OPENSHELL_GATEWAY_ENDPOINT) rather than the --gateway-endpoint flag, since
// that's literally how the onboarding thread's service account configured
// it.
func TestTokenShow_GatewayEndpointEnvVar_TrailingSlashMatchesDefaultPort(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "config")
	gwDir := filepath.Join(xdg, "openshell", "gateways", "rosa")
	if err := os.MkdirAll(gwDir, 0o700); err != nil {
		t.Fatal(err)
	}
	md := `{"name":"rosa","gateway_endpoint":"https://gw.example.com:443","is_remote":true,"gateway_port":0,"auth_mode":"oidc","oidc_issuer":"https://issuer"}`
	if err := os.WriteFile(filepath.Join(gwDir, "metadata.json"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle := `{"access_token":"tok","expires_at":9999999999,"issuer":"https://issuer","client_id":"openshell-cli"}`
	if err := os.WriteFile(filepath.Join(gwDir, "oidc_token.json"), []byte(bundle), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("OPENSHELL_GATEWAY_ENDPOINT", "https://gw.example.com/")

	out, err := runCmd(t, "token", "show")
	if err != nil {
		t.Fatalf("token show: %v", err)
	}
	if !strings.Contains(out, "gateway=rosa") {
		t.Errorf("expected a gateway-backed source, got:\n%s", out)
	}
}
