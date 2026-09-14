package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
