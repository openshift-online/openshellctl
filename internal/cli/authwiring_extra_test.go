package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
)

func TestOIDCConfigFetcher(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/oidc-config" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"issuer":"https://issuer","audience":"openshell-cli"}`))
	}))
	defer srv.Close()

	iss, aud, err := oidcConfigFetcher(context.Background(), srv.URL)
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
	if _, _, err := oidcConfigFetcher(context.Background(), srv.URL); err == nil {
		t.Error("expected an error for non-200")
	}
}

// TestOIDCConfigFetcher_UntrustedCertFailsByDefault confirms the discovery
// probe performs real TLS verification by default — a self-signed/
// internal-CA cert (common for staging gateways) is rejected, just like any
// other HTTPS client, unless --gateway-insecure opts out of it.
func TestOIDCConfigFetcher_UntrustedCertFailsByDefault(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"issuer":"https://issuer","audience":"openshell-cli"}`))
	}))
	defer srv.Close()

	_, _, err := oidcConfigFetcher(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected a certificate verification error against an untrusted self-signed cert")
	}
	if !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "x509") {
		t.Errorf("expected a certificate-verification error, got: %v", err)
	}
}

// TestOIDCConfigFetcher_GatewayInsecureSkipsVerification confirms
// --gateway-insecure (the same flag the real gRPC dial already honors, see
// pkg/gateway/dial.go) also lets the discovery probe skip TLS verification —
// without it, a gateway add against a self-signed/internal-CA staging
// gateway can never succeed, since no gateway is registered yet at the
// discovery step for a per-gateway CA bundle to apply.
// TestOIDCConfigFetcher_GatewayInsecureSkipsVerification confirms
// oidcConfigFetcher inherits the global --gateway-insecure transport
// override (applyGatewayInsecureTransport, root.go) rather than keeping its
// own separate per-call override — one place decides "skip verification,"
// not two that could drift (the SDK's own OIDC HTTP client needs the global
// override regardless, since it has no per-call injection point at all; see
// root.go's applyGatewayInsecureTransport doc comment).
func TestOIDCConfigFetcher_GatewayInsecureSkipsVerification(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"issuer":"https://issuer","audience":"openshell-cli"}`))
	}))
	defer srv.Close()

	viper.Set("gateway-insecure", true)
	applyGatewayInsecureTransport()
	iss, aud, err := oidcConfigFetcher(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch with --gateway-insecure: %v", err)
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

func TestUserAgent(t *testing.T) {
	ua := userAgent()
	if !strings.HasPrefix(ua, "openshellctl/") || !strings.Contains(ua, "openshell-pin") {
		t.Errorf("user agent = %q", ua)
	}
}
