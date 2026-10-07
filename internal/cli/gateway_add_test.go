package cli

import (
	"context"
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

func oidcDiscoveryServer(t *testing.T, issuer, audience string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/oidc-config" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"issuer":"` + issuer + `","audience":"` + audience + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func notOIDCServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
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
