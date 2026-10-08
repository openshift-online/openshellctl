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
//
// Uses a loopback endpoint (127.0.0.1), not a DNS name: `token show`'s
// reportCurrentUser does a best-effort whoami dial that cliDeps injection
// cannot stand in for here (injecting a mock Gateway would also bypass
// resolveAuth's real gatewayconfig.Resolve/FindByEndpoint path, which is
// exactly what this test needs to exercise for real). A loopback address
// with nothing listening fails the dial immediately with a local connection
// refusal — no DNS lookup, no real network egress — keeping the test
// hermetic without the dial itself touching the network.
func TestTokenShow_GatewayEndpointWithTrailingSlashMatchesRegisteredDefaultPort(t *testing.T) {
	// Registered the way `gateway add`/the CronJobs do: an explicit default
	// port.
	xdg := setupGatewayTreeWithEndpoint(t, "https://127.0.0.1:443")
	writeOIDCBundle(t, xdg, "rosa")
	// No active_gateway shortcut: this must be found purely by
	// FindByEndpoint's full scan, not the active-gateway fast path
	// FindByEndpoint also checks first (setupGatewayTreeWithEndpoint sets
	// one by default; remove it so this test exercises the scan loop).
	if err := os.Remove(filepath.Join(xdg, "openshell", "active_gateway")); err != nil {
		t.Fatal(err)
	}

	// Exported the way a service account would: trailing slash, no port.
	out, err := runCmd(t, "token", "show", "--gateway-endpoint", "https://127.0.0.1/")
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
// it. Keeps the default active_gateway from setupGatewayTreeWithEndpoint
// (a realistic state: a service account may well have a previously-selected
// active gateway too), exercising FindByEndpoint's active-gateway fast path
// instead of the full-scan path the test above covers.
func TestTokenShow_GatewayEndpointEnvVar_TrailingSlashMatchesDefaultPort(t *testing.T) {
	xdg := setupGatewayTreeWithEndpoint(t, "https://127.0.0.1:443")
	writeOIDCBundle(t, xdg, "rosa")
	t.Setenv("OPENSHELL_GATEWAY_ENDPOINT", "https://127.0.0.1/")

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
