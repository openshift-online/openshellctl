package cli

import (
	"context"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
)

func TestOIDCConfigURL(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  string
		wantURL   string
		wantErr   bool
		wantInsec bool
	}{
		{"https ok", "https://gw", "https://gw/auth/oidc-config", false, false},
		{"http loopback IPv4 ok", "http://127.0.0.1:8080", "http://127.0.0.1:8080/auth/oidc-config", false, false},
		{"http localhost ok", "http://localhost", "http://localhost/auth/oidc-config", false, false},
		{"http loopback IPv6 ok", "http://[::1]:80", "http://[::1]:80/auth/oidc-config", false, false},
		{"http non-loopback rejected", "http://gw.example.com", "", true, true},
		{"ftp scheme rejected", "ftp://gw", "", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := oidcConfigURL(tt.endpoint)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("oidcConfigURL(%q) = %q, nil; want error", tt.endpoint, got)
				}
				var insec *InsecureOIDCConfigURLError
				if tt.wantInsec && !errors.As(err, &insec) {
					t.Errorf("oidcConfigURL(%q): err = %v, want *InsecureOIDCConfigURLError", tt.endpoint, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("oidcConfigURL(%q): unexpected error: %v", tt.endpoint, err)
			}
			if got != tt.wantURL {
				t.Errorf("oidcConfigURL(%q) = %q, want %q", tt.endpoint, got, tt.wantURL)
			}
		})
	}
}

func TestOIDCConfigFetcher(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/oidc-config" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"issuer":"https://issuer","audience":"openshell-cli"}`))
	}))
	defer srv.Close()

	fetcher := newOIDCConfigFetcher(nil)
	iss, aud, err := fetcher(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if iss != "https://issuer" || aud != "openshell-cli" {
		t.Errorf("got issuer=%q audience=%q", iss, aud)
	}
}

func TestOIDCConfigFetcher_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	fetcher := newOIDCConfigFetcher(nil)
	if _, _, err := fetcher(context.Background(), srv.URL); err == nil {
		t.Error("expected an error for non-200")
	}
}

// TestOIDCConfigFetcher_UntrustedCertFailsByDefault confirms that the fetcher
// (with nil TLS config — system roots) rejects a self-signed cert, so
// discovery against a staging gateway without an explicit CA fails fast and
// tells the user to pass --oidc-issuer.
func TestOIDCConfigFetcher_UntrustedCertFailsByDefault(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"issuer":"https://issuer","audience":"openshell-cli"}`))
	}))
	defer srv.Close()

	fetcher := newOIDCConfigFetcher(nil)
	_, _, err := fetcher(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected a certificate verification error against an untrusted self-signed cert")
	}
	if !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "x509") {
		t.Errorf("expected a certificate-verification error, got: %v", err)
	}
}

// TestOIDCConfigFetcher_PlainHTTPNonLoopbackRejected confirms that the
// fetcher rejects a non-loopback http:// endpoint before making any request.
func TestOIDCConfigFetcher_PlainHTTPNonLoopbackRejected(t *testing.T) {
	fetcher := newOIDCConfigFetcher(nil)
	_, _, err := fetcher(context.Background(), "http://gw.example.com:8080")
	if err == nil {
		t.Fatal("expected error for non-loopback http endpoint")
	}
	var insec *InsecureOIDCConfigURLError
	if !errors.As(err, &insec) {
		t.Errorf("expected *InsecureOIDCConfigURLError, got %T: %v", err, err)
	}
}

// TestOIDCConfigFetcher_DoesNotFollowRedirect confirms that a 302 response is
// not followed — the client returns an error for the non-200 status and
// never contacts the redirect target.
func TestOIDCConfigFetcher_DoesNotFollowRedirect(t *testing.T) {
	second := 0
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		second++
		_, _ = w.Write([]byte(`{"issuer":"https://issuer","audience":"openshell-cli"}`))
	}))
	defer dest.Close()

	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL+r.URL.Path, http.StatusFound)
	}))
	defer redir.Close()

	fetcher := newOIDCConfigFetcher(nil)
	_, _, err := fetcher(context.Background(), redir.URL)
	if err == nil {
		t.Fatal("expected error (302 not followed → non-200)")
	}
	if second != 0 {
		t.Errorf("redirect target was contacted %d times, want 0", second)
	}
}

// TestOIDCConfigFetcher_UsesGatewayCA confirms that the fetcher trusts the
// gateway's CA certificate (via gateway.ClientTLSConfig) and succeeds against
// an https server whose cert is signed by that CA.
func TestOIDCConfigFetcher_UsesGatewayCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/oidc-config" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"issuer":"https://issuer","audience":"openshell-cli"}`))
	}))
	defer srv.Close()

	caDir := t.TempDir()
	caFile := filepath.Join(caDir, "ca.crt")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(caFile, certPEM, 0o600); err != nil {
		t.Fatalf("write CA: %v", err)
	}

	tlsCfg, err := gateway.ClientTLSConfig(gateway.DialConfig{
		TLS:     gatewayconfig.TLSMaterial{Present: true, CAFile: "ca.crt"},
		TLSRoot: caDir,
	})
	if err != nil {
		t.Fatalf("ClientTLSConfig: %v", err)
	}

	fetcher := newOIDCConfigFetcher(tlsCfg)
	iss, aud, err := fetcher(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch with gateway CA: %v", err)
	}
	if iss != "https://issuer" || aud != "openshell-cli" {
		t.Errorf("got issuer=%q audience=%q", iss, aud)
	}
}

func TestAuthWriterAdapter_RootsAtGatewayDir(t *testing.T) {
	root := t.TempDir()
	a := authWriterAdapter{w: &gatewayconfig.OSWriter{Root: root}, gatewayName: "rosa"}
	if err := a.WriteFile("oidc_token.json", []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "gateways", "rosa", "oidc_token.json"))
	if err != nil {
		t.Fatalf("expected file under gateways/rosa/: %v", err)
	}
	if string(got) != "data" {
		t.Errorf("content = %q", string(got))
	}
}

func TestTokenWriterFor_NilForEndpointOnly(t *testing.T) {
	w, err := tokenWriterFor(&gatewayconfig.Target{Name: "", Endpoint: "https://x"})
	if err != nil {
		t.Fatal(err)
	}
	if w != nil {
		t.Error("endpoint-only target should yield no writer")
	}
}

// TestRequireTokenWriter_EndpointOnly_UsageError confirms requireTokenWriter
// returns a typed usage error (naming `gateway add`) instead of
// tokenWriterFor's silent (nil, nil) — the fix for --write's swallowed
// warning-then-exit-0 bug.
func TestRequireTokenWriter_EndpointOnly_UsageError(t *testing.T) {
	_, err := requireTokenWriter(&gatewayconfig.Target{Name: "", Endpoint: "https://x"})
	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v, want UsageError", err)
	}
	if !strings.Contains(err.Error(), "gateway add") {
		t.Errorf("error should name `gateway add`, got: %v", err)
	}
}

// TestRequireTokenWriter_NilTarget_UsageError confirms a nil target (no
// gateway resolved at all) is treated the same way.
func TestRequireTokenWriter_NilTarget_UsageError(t *testing.T) {
	_, err := requireTokenWriter(nil)
	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v, want UsageError", err)
	}
}

// TestRequireTokenWriter_NamedResolvedGateway_Succeeds confirms a real,
// registered gateway still gets a working writer (requireTokenWriter is not
// stricter than tokenWriterFor in the success case, only in the failure
// case).
func TestRequireTokenWriter_NamedResolvedGateway_Succeeds(t *testing.T) {
	root := t.TempDir()
	target := &gatewayconfig.Target{
		Name:     "rosa",
		Endpoint: "https://gw",
		Resolved: &gatewayconfig.Resolved{Name: "rosa"},
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	w, err := requireTokenWriter(target)
	if err != nil {
		t.Fatalf("requireTokenWriter: %v", err)
	}
	if w == nil {
		t.Error("expected a non-nil writer for a named, resolved gateway")
	}
}

// TestAuthWriterAdapter_RejectsTraversal confirms authWriterAdapter.WriteFile
// returns an error when gatewayName contains path traversal characters rather
// than writing a file outside the config root.
func TestAuthWriterAdapter_RejectsTraversal(t *testing.T) {
	root := t.TempDir()
	a := authWriterAdapter{w: &gatewayconfig.OSWriter{Root: root}, gatewayName: "../../x"}
	err := a.WriteFile("oidc_token.json", []byte("data"), 0o600)
	if err == nil {
		t.Fatal("expected error for traversal gatewayName, got nil")
	}
	var inv *gatewayconfig.InvalidGatewayNameError
	if !errors.As(err, &inv) {
		t.Errorf("err = %T(%v), want *InvalidGatewayNameError", err, err)
	}
	// Confirm nothing was written outside root.
	parent := filepath.Dir(root)
	xPath := filepath.Join(parent, "x", "oidc_token.json")
	if _, statErr := os.Stat(xPath); statErr == nil {
		t.Errorf("file should not exist at %s", xPath)
	}
}

// TestTokenWriterFor_UsesSlug confirms tokenWriterFor writes the token under
// the directory slug (Resolved.Name), not the display name in Target.Name.
func TestTokenWriterFor_UsesSlug(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	target := &gatewayconfig.Target{
		Name:     "ROSA Agentic Devx",
		Endpoint: "https://gw",
		Resolved: &gatewayconfig.Resolved{Name: "rosa-agentic-devx"},
	}
	w, err := tokenWriterFor(target)
	if err != nil {
		t.Fatalf("tokenWriterFor: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil writer for resolved gateway")
	}
	if err := w.WriteFile("oidc_token.json", []byte("tok"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Token must land under the slug, not the display name.
	slugPath := filepath.Join(root, "openshell", "gateways", "rosa-agentic-devx", "oidc_token.json")
	if _, statErr := os.Stat(slugPath); statErr != nil {
		t.Errorf("expected token at %s, got: %v", slugPath, statErr)
	}
	displayPath := filepath.Join(root, "openshell", "gateways", "ROSA Agentic Devx", "oidc_token.json")
	if _, statErr := os.Stat(displayPath); statErr == nil {
		t.Errorf("token should NOT be at display-name path %s", displayPath)
	}
}

func TestUserAgent(t *testing.T) {
	ua := userAgent()
	if !strings.HasPrefix(ua, "openshellctl/") || !strings.Contains(ua, "openshell-pin") {
		t.Errorf("user agent = %q", ua)
	}
}

// TestWriteTokenFlag_EndpointOnlyTarget_UsageError confirms --write-token (the
// persistent flag honored by every command that resolves auth via
// resolveTokenSource, not just `token refresh --write`) now fails loudly
// against an endpoint-only target instead of silently no-oping — the same
// swallowed-warning shape this story fixed for `token refresh --write`,
// caught by PR review on a different command surface. No real dial happens:
// resolveTokenSource returns the usage error before auth.Resolve/dial are
// ever reached.
func TestWriteTokenFlag_EndpointOnlyTarget_UsageError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runCmd(t, "sandbox", "list", "--write-token", "--gateway-endpoint", "https://nowhere.invalid")
	if err == nil {
		t.Fatal("expected an error")
	}
	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v, want UsageError", err)
	}
	if !strings.Contains(err.Error(), "gateway add") {
		t.Errorf("error should name `gateway add`, got: %v", err)
	}
}

func TestIssuerOverrideWarning(t *testing.T) {
	pstr := func(s string) *string { return &s }
	tests := []struct {
		name           string
		override       string
		metadataIssuer *string
		wantEmpty      bool
	}{
		{"no override", "", pstr("https://issuer"), true},
		{"same value", "https://issuer", pstr("https://issuer"), true},
		{"same with trailing slash on override", "https://issuer/", pstr("https://issuer"), true},
		{"differing issuers", "https://other", pstr("https://issuer"), false},
		{"nil metadata issuer", "https://issuer", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := issuerOverrideWarning(tt.override, tt.metadataIssuer)
			if tt.wantEmpty && got != "" {
				t.Errorf("issuerOverrideWarning(%q, %v) = %q, want empty", tt.override, tt.metadataIssuer, got)
			}
			if !tt.wantEmpty && got == "" {
				t.Errorf("issuerOverrideWarning(%q, %v) = empty, want non-empty warning", tt.override, tt.metadataIssuer)
			}
		})
	}
}
