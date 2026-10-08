package cli

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupDoctorGatewayTree registers a gateway named "rosa" at the given
// endpoint, with an oidc_token.json bundle built from jwt. Reuses
// setupGatewayTreeWithEndpoint (tokenflow_test.go) rather than hand-rolling
// the tree.
func setupDoctorGatewayTree(t *testing.T, endpoint, jwt string) {
	t.Helper()
	xdg := setupGatewayTreeWithEndpoint(t, endpoint)
	bundle := `{"access_token":"` + jwt + `","expires_at":9999999999,"issuer":"https://issuer","client_id":"openshell-cli"}`
	if err := os.WriteFile(filepath.Join(xdg, "openshell", "gateways", "rosa", "oidc_token.json"), []byte(bundle), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestDoctorCommand_AllGreen runs `doctor` end to end through the real
// command tree (no cliDeps injection — real on-disk gateway tree, a real
// JWT bundle, and a real httptest.Server standing in for the gateway's
// /auth/oidc-config endpoint) and confirms every check passes.
func TestDoctorCommand_AllGreen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/oidc-config" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"issuer":"https://issuer","audience":"openshell-cli"}`))
	}))
	defer srv.Close()

	setupDoctorGatewayTree(t, srv.URL, staticJWT(t))

	out, err := runCmd(t, "doctor", "--gateway-endpoint", srv.URL)
	if err != nil {
		t.Fatalf("doctor: %v\noutput:\n%s", err, out)
	}
	for _, want := range []string{"Endpoint URL", "DNS", "HTTP reachability", "Credentials", "Audience", "Roles", "Expiry"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing check %q; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "✗") {
		t.Errorf("expected no failures, got:\n%s", out)
	}
}

// TestDoctorCommand_NoCredentials_ChecksSkippedAndExit3 is the ticket's
// second acceptance criterion: with the secret unset (and no disk bundle),
// the Credentials check shows the checked-sources list, downstream checks
// are skipped, and the exit code is 3.
func TestDoctorCommand_NoCredentials_ChecksSkippedAndExit3(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"issuer":"https://issuer","audience":"openshell-cli"}`))
	}))
	defer srv.Close()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out, err := runCmd(t, "doctor", "--gateway-endpoint", srv.URL)
	if err == nil {
		t.Fatalf("expected an error, got success:\n%s", out)
	}
	if exitCodeFor(err) != ExitAuth {
		t.Errorf("exit = %d, want ExitAuth (3)", exitCodeFor(err))
	}
	if !strings.Contains(out, "OPENSHELL_OIDC_CLIENT_SECRET") {
		t.Errorf("Credentials line should show the checked-sources list; got:\n%s", out)
	}
	if !strings.Contains(out, "Hint:") {
		t.Errorf("a failing check should print a copy-pasteable next step; got:\n%s", out)
	}
	for _, name := range []string{"Audience", "Roles", "Expiry"} {
		if !strings.Contains(out, name) {
			t.Errorf("output should still list the skipped %q check; got:\n%s", name, out)
		}
	}
}

// TestDoctorCommand_OutputJSON confirms -o json renders the full check list
// as a real JSON array. Uses a loopback endpoint, not a DNS name: the DNS
// check does a real net.DefaultResolver lookup (CheckDNS has no test-only
// injection seam at the CLI layer — doctor.go always wires the real
// resolver), and an unresolvable DNS name would mean an actual, slow
// network query in this sandbox; a literal IP resolves immediately with no
// real lookup at all.
func TestDoctorCommand_OutputJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	out, err := runCmd(t, "doctor", "--gateway-endpoint", "https://127.0.0.1:1", "-o", "json")
	if err == nil {
		t.Fatal("expected an error (no credentials), but still expect valid JSON output")
	}
	var parsed []map[string]any
	if jsonErr := json.Unmarshal([]byte(out), &parsed); jsonErr != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", jsonErr, out)
	}
	if len(parsed) == 0 {
		t.Fatal("expected at least one check in the JSON output")
	}
}

// noRoleJWT builds a JWT (same shape as staticJWTWithAudience,
// refresh_honesty_test.go) but deliberately omitting realm_access.roles —
// the exact "authenticated, but lacks the required role" scenario the Roles
// check exists to catch.
func noRoleJWT(t *testing.T, aud string) string {
	t.Helper()
	enc := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	payload := map[string]any{
		"iss": "https://issuer",
		"sub": "user-9",
		"aud": aud,
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	}
	return enc(map[string]any{"alg": "RS256"}) + "." + enc(payload) + ".c2ln"
}

// TestDoctorCommand_RolesFail_Exit7 confirms a token with no gateway role
// fails the Roles check with exit code 7 (the ticket's third acceptance
// criterion — "unit test with a synthetic JWT payload").
func TestDoctorCommand_RolesFail_Exit7(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"issuer":"https://issuer","audience":"openshell-cli"}`))
	}))
	defer srv.Close()

	setupDoctorGatewayTree(t, srv.URL, noRoleJWT(t, "openshell-cli"))

	out, err := runCmd(t, "doctor", "--gateway-endpoint", srv.URL)
	if err == nil {
		t.Fatalf("expected an error, got success:\n%s", out)
	}
	if exitCodeFor(err) != ExitForbidden {
		t.Errorf("exit = %d, want ExitForbidden (7)", exitCodeFor(err))
	}
	if !strings.Contains(out, "openshell-user") {
		t.Errorf("Roles failure should name the required role; got:\n%s", out)
	}
}
