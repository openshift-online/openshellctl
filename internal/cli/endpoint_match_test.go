package cli

import (
	"encoding/json"
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
// Runs with -o json, not the default text output: text mode's
// reportCurrentUser does a best-effort whoami dial that cliDeps injection
// cannot stand in for here (injecting a mock Gateway would also bypass
// resolveAuth's real gatewayconfig.Resolve/FindByEndpoint path, which is
// exactly what this test needs to exercise for real). reportCurrentUser
// returns immediately for any non-text output (see token.go), and the JSON
// payload already includes the resolved source's describe string — so -o
// json gives the exact same assertion with no dial at all, not merely a
// fast-failing loopback one.
func TestTokenShow_GatewayEndpointWithTrailingSlashMatchesRegisteredDefaultPort(t *testing.T) {
	// Registered the way `gateway add`/the CronJobs do: an explicit default
	// port.
	xdg := setupGatewayTreeWithEndpoint(t, "https://gw.example.com:443")
	writeOIDCBundle(t, xdg, "rosa")
	// No active_gateway shortcut: this must be found purely by
	// FindByEndpoint's full scan, not the active-gateway fast path
	// FindByEndpoint also checks first (setupGatewayTreeWithEndpoint sets
	// one by default; remove it so this test exercises the scan loop).
	if err := os.Remove(filepath.Join(xdg, "openshell", "active_gateway")); err != nil {
		t.Fatal(err)
	}

	// Exported the way a service account would: trailing slash, no port.
	out, err := runCmd(t, "token", "show", "-o", "json", "--gateway-endpoint", "https://gw.example.com/")
	if err != nil {
		t.Fatalf("token show: %v", err)
	}
	assertGatewayBackedJSON(t, out, "rosa")
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
	xdg := setupGatewayTreeWithEndpoint(t, "https://gw.example.com:443")
	writeOIDCBundle(t, xdg, "rosa")
	t.Setenv("OPENSHELL_GATEWAY_ENDPOINT", "https://gw.example.com/")

	out, err := runCmd(t, "token", "show", "-o", "json")
	if err != nil {
		t.Fatalf("token show: %v", err)
	}
	assertGatewayBackedJSON(t, out, "rosa")
}

// assertGatewayBackedJSON parses token show's -o json output and confirms
// its "describe" field names gwName as the source gateway (the
// DiskBundleSource.Describe() format is "oidc_token.json (gateway=<name>)").
func assertGatewayBackedJSON(t *testing.T, out, gwName string) {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	describe, _ := parsed["describe"].(string)
	want := "gateway=" + gwName
	if !strings.Contains(describe, want) {
		t.Errorf("describe = %q, want it to contain %q (a gateway-backed oidc_token.json source)", describe, want)
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
