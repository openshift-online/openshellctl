package cli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openshift-online/openshellctl/pkg/auth"
)

// fakeTokenSource is a minimal auth.TokenSource test double — gateway add's
// client-credentials auth step is exercised hermetically by injecting one of
// these via cliDeps rather than letting auth.Resolve build a real
// ClientCredentialsSource that would dial out to an issuer.
type fakeTokenSource struct {
	tok *auth.Token
	err error
}

func (f fakeTokenSource) Token(context.Context) (*auth.Token, error) { return f.tok, f.err }
func (f fakeTokenSource) Invalidate()                                {}
func (f fakeTokenSource) Describe() string                           { return "fake" }

// trustTestServerTLS swaps http.DefaultTransport (what oidcConfigFetcher's
// internal http.Client uses, since it sets no Transport of its own) for one
// that trusts srv's self-signed cert, restoring the original on cleanup —
// needed because gateway add now requires https (NewMetadata rejects
// plaintext endpoints; see TestNewMetadata_RejectsNonHTTPSEndpoint), so these
// discovery-probe fixtures must be real TLS servers, not plain HTTP ones.
func trustTestServerTLS(t *testing.T, srv *httptest.Server) {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	original := http.DefaultTransport
	http.DefaultTransport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}
	t.Cleanup(func() { http.DefaultTransport = original })
}

func oidcDiscoveryServer(t *testing.T, issuer, audience string) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/oidc-config" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"issuer":"` + issuer + `","audience":"` + audience + `"}`))
	}))
	t.Cleanup(srv.Close)
	trustTestServerTLS(t, srv)
	return srv
}

func notOIDCServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	trustTestServerTLS(t, srv)
	return srv
}

func TestGatewayAdd_DiscoversOIDCAndAuthenticates(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("OPENSHELL_OIDC_CLIENT_SECRET", "s3cr3t")

	srv := oidcDiscoveryServer(t, "https://issuer.example.com", "aud-1")

	src := fakeTokenSource{tok: &auth.Token{
		AccessToken: "tok",
		Subject:     "svc-account",
		Expiry:      time.Now().Add(time.Hour),
	}}
	out, err := runCmdWithGateway(t, cliDeps{TokenSource: src}, "gateway", "add", srv.URL, "--name", "test-gw")
	if err != nil {
		t.Fatalf("gateway add: %v", err)
	}
	if !strings.Contains(out, "✓ Gateway 'test-gw' added and set as active") {
		t.Errorf("output missing registration confirmation; got:\n%s", out)
	}
	if !strings.Contains(out, "✓ Authenticated via client credentials") {
		t.Errorf("output missing auth confirmation; got:\n%s", out)
	}

	// metadata.json should exist and name the discovered issuer.
	data, rerr := os.ReadFile(filepath.Join(xdg, "openshell", "gateways", "test-gw", "metadata.json"))
	if rerr != nil {
		t.Fatalf("metadata.json not written: %v", rerr)
	}
	if !strings.Contains(string(data), "https://issuer.example.com") {
		t.Errorf("metadata.json missing discovered issuer:\n%s", data)
	}

	// oidc_token.json should exist with a real expires_at (unlike the CronJob's
	// Python writer, which omits it).
	tokData, rerr := os.ReadFile(filepath.Join(xdg, "openshell", "gateways", "test-gw", "oidc_token.json"))
	if rerr != nil {
		t.Fatalf("oidc_token.json not written: %v", rerr)
	}
	if !strings.Contains(string(tokData), "expires_at") {
		t.Errorf("oidc_token.json missing expires_at:\n%s", tokData)
	}

	// active_gateway should point at the new gateway.
	active, rerr := os.ReadFile(filepath.Join(xdg, "openshell", "active_gateway"))
	if rerr != nil || string(active) != "test-gw" {
		t.Errorf("active_gateway = %q, err=%v, want test-gw", active, rerr)
	}
}

func TestGatewayAdd_ExplicitIssuerSkipsDiscovery(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENSHELL_NO_BROWSER", "1") // avoid the browser-login fallback with no secret

	// No discovery server at all — an unreachable host would fail if dialed.
	out, err := runCmd(t, "gateway", "add", "https://gw.example.invalid", "--name", "test-gw", "--oidc-issuer", "https://issuer")
	if err != nil {
		t.Fatalf("gateway add: %v", err)
	}
	if !strings.Contains(out, "added and set as active") {
		t.Errorf("output: %s", out)
	}
}

func TestGatewayAdd_EdgeGatewayUnsupported(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := notOIDCServer(t)

	_, err := runCmd(t, "gateway", "add", srv.URL, "--name", "edge-gw")
	if err == nil {
		t.Fatal("expected an error for a non-OIDC gateway")
	}
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want usage", exitCodeFor(err))
	}
	if !strings.Contains(err.Error(), "--oidc-issuer") {
		t.Errorf("error should hint at --oidc-issuer, got: %v", err)
	}
}

func TestGatewayAdd_AlreadyExists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENSHELL_NO_BROWSER", "1")

	_, err := runCmd(t, "gateway", "add", "https://gw.example.com", "--name", "dup", "--oidc-issuer", "https://issuer")
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	_, err = runCmd(t, "gateway", "add", "https://gw.example.com", "--name", "dup", "--oidc-issuer", "https://issuer")
	if err == nil {
		t.Fatal("expected an error registering the same name twice")
	}
	if exitCodeFor(err) != ExitConflict {
		t.Errorf("exit = %d, want conflict", exitCodeFor(err))
	}
}

func TestGatewayAdd_NoSecretNoBrowser_RegistersAndHints(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENSHELL_NO_BROWSER", "1")

	out, err := runCmd(t, "gateway", "add", "https://gw.example.com", "--name", "svc-gw", "--oidc-issuer", "https://issuer")
	if err != nil {
		t.Fatalf("gateway add: %v", err)
	}
	if !strings.Contains(out, "added and set as active") {
		t.Errorf("output missing registration confirmation: %s", out)
	}
	if !strings.Contains(out, "gateway login") {
		t.Errorf("output should hint at gateway login, got: %s", out)
	}
}

func TestGatewayAdd_InvalidEndpoint(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runCmd(t, "gateway", "add", "")
	if err == nil {
		t.Fatal("expected an error for an empty endpoint")
	}
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want usage", exitCodeFor(err))
	}
}

// TestGatewayAdd_Force confirms --force overwrites an existing registration
// instead of GatewayExistsError.
func TestGatewayAdd_Force(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENSHELL_NO_BROWSER", "1")

	if _, err := runCmd(t, "gateway", "add", "https://old.example.com", "--name", "dup", "--oidc-issuer", "https://issuer"); err != nil {
		t.Fatalf("first add: %v", err)
	}
	out, err := runCmd(t, "gateway", "add", "https://new.example.com", "--name", "dup", "--oidc-issuer", "https://issuer", "--force")
	if err != nil {
		t.Fatalf("forced add: %v", err)
	}
	if !strings.Contains(out, "added and set as active") {
		t.Errorf("out = %s", out)
	}
	listOut, err := runCmd(t, "gateway", "list", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listOut, "new.example.com") {
		t.Errorf("force should have overwritten the endpoint, got: %s", listOut)
	}
}

// TestGatewayAdd_NoBrowserFalseValueStillOpensLogin confirms
// OPENSHELL_NO_BROWSER=0/false is treated as false (browser allowed), not as
// "set" the way a naive non-empty-string check would.
func TestGatewayAdd_NoBrowserFalseValueFallsBackToLogin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENSHELL_NO_BROWSER", "0")
	// No secret and no-browser=false -> attempts the real browser login flow,
	// which fails in this headless test environment (no browser, no
	// resolvable OIDC endpoint) rather than taking the register-only path.
	out, err := runCmd(t, "gateway", "add", "https://gw.example.com", "--name", "svc-gw", "--oidc-issuer", "https://issuer")
	if err == nil {
		t.Fatalf("expected the browser login fallback to fail in this test env, got success:\n%s", out)
	}
	if strings.Contains(out, "No client secret found") {
		t.Errorf("OPENSHELL_NO_BROWSER=0 should not take the register-only hint path, got: %s", out)
	}
}

// TestGatewayAdd_WarnsOnDefaultedClientIDWithSecret confirms a warning is
// printed when a client secret is present but --oidc-client-id wasn't given
// (metadata.json ends up with the "openshell-cli" default, which may not
// match the service account's real client id).
func TestGatewayAdd_WarnsOnDefaultedClientIDWithSecret(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENSHELL_OIDC_CLIENT_SECRET", "s3cr3t")

	src := fakeTokenSource{tok: &auth.Token{AccessToken: "tok", Expiry: time.Now().Add(time.Hour)}}
	out, err := runCmdWithGateway(t, cliDeps{TokenSource: src}, "gateway", "add", "https://gw.example.com", "--name", "test-gw", "--oidc-issuer", "https://issuer")
	if err != nil {
		t.Fatalf("gateway add: %v", err)
	}
	if !strings.Contains(out, "no --oidc-client-id given") {
		t.Errorf("expected a warning about the defaulted client id, got: %s", out)
	}
}

// TestGatewayAdd_AuthFailureRollsBack confirms a failed authentication
// attempt doesn't leave a half-registered, active gateway behind: the
// metadata.json just written is removed and the previously-active gateway
// (if any) is restored, so a corrected retry doesn't hit GatewayExistsError.
func TestGatewayAdd_AuthFailureRollsBack(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	// A pre-existing active gateway that must survive the failed attempt.
	t.Setenv("OPENSHELL_NO_BROWSER", "1")
	if _, err := runCmd(t, "gateway", "add", "https://other.example.com", "--name", "other", "--oidc-issuer", "https://issuer"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Setenv("OPENSHELL_OIDC_CLIENT_SECRET", "wrong-secret")
	failing := fakeTokenSource{err: errors.New("invalid_client")}
	_, err := runCmdWithGateway(t, cliDeps{TokenSource: failing}, "gateway", "add", "https://gw.example.com", "--name", "bad-gw", "--oidc-issuer", "https://issuer")
	if err == nil {
		t.Fatal("expected the auth failure to propagate")
	}

	if _, statErr := os.Stat(filepath.Join(xdg, "openshell", "gateways", "bad-gw", "metadata.json")); statErr == nil {
		t.Error("metadata.json should have been rolled back after an auth failure")
	}
	active, rerr := os.ReadFile(filepath.Join(xdg, "openshell", "active_gateway"))
	if rerr != nil || string(active) != "other" {
		t.Errorf("active_gateway = %q, err=%v, want restored to 'other'", active, rerr)
	}

	// A corrected retry must not hit GatewayExistsError.
	ok := fakeTokenSource{tok: &auth.Token{AccessToken: "tok", Expiry: time.Now().Add(time.Hour)}}
	_, err = runCmdWithGateway(t, cliDeps{TokenSource: ok}, "gateway", "add", "https://gw.example.com", "--name", "bad-gw", "--oidc-issuer", "https://issuer")
	if err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
}

// TestGatewayAdd_AuthFailureRollsBack_NoPriorActive confirms rollback clears
// active_gateway entirely (rather than leaving the failed gateway active)
// when there was no previously-active gateway.
func TestGatewayAdd_AuthFailureRollsBack_NoPriorActive(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("OPENSHELL_OIDC_CLIENT_SECRET", "wrong-secret")

	failing := fakeTokenSource{err: errors.New("invalid_client")}
	_, err := runCmdWithGateway(t, cliDeps{TokenSource: failing}, "gateway", "add", "https://gw.example.com", "--name", "bad-gw", "--oidc-issuer", "https://issuer")
	if err == nil {
		t.Fatal("expected the auth failure to propagate")
	}
	if _, statErr := os.Stat(filepath.Join(xdg, "openshell", "active_gateway")); statErr == nil {
		t.Error("active_gateway should not exist when there was no prior active gateway")
	}
}
