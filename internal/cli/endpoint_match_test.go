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
//
// Built on setupGatewayTreeWithEndpoint (tokenflow_test.go), the same
// fixture family TestTokenShow_StaticToken/TestWhoami_* already use, per
// this story's acceptance criterion ("CLI test using the setupGatewayTree
// fixture") — parameterized with the explicit :443 this scenario needs,
// since setupGatewayTree's own default (a bare host, no port) wouldn't
// exercise the bug.
func TestTokenShow_GatewayEndpointWithTrailingSlashMatchesRegisteredDefaultPort(t *testing.T) {
	// Registered the way `gateway add`/the CronJobs do: an explicit default
	// port.
	xdg := setupGatewayTreeWithEndpoint(t, "https://gw.example.com:443")
	writeOIDCBundle(t, xdg, "rosa")

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
	xdg := setupGatewayTreeWithEndpoint(t, "https://gw.example.com:443")
	writeOIDCBundle(t, xdg, "rosa")
	t.Setenv("OPENSHELL_GATEWAY_ENDPOINT", "https://gw.example.com/")

	out, err := runCmd(t, "token", "show")
	if err != nil {
		t.Fatalf("token show: %v", err)
	}
	if !strings.Contains(out, "gateway=rosa") {
		t.Errorf("expected a gateway-backed source, got:\n%s", out)
	}
}

// writeOIDCBundle writes a valid, non-expired oidc_token.json for gwName
// into xdg (as returned by setupGatewayTree/setupGatewayTreeWithEndpoint),
// so a `token show` against it resolves a real gateway-backed
// DiskBundleSource instead of failing on a missing bundle file.
func writeOIDCBundle(t *testing.T, xdg, gwName string) {
	t.Helper()
	bundle := `{"access_token":"tok","expires_at":9999999999,"issuer":"https://issuer","client_id":"openshell-cli"}`
	path := filepath.Join(xdg, "openshell", "gateways", gwName, "oidc_token.json")
	if err := os.WriteFile(path, []byte(bundle), 0o600); err != nil {
		t.Fatal(err)
	}
}
