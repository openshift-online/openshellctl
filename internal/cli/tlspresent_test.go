package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift-online/openshellctl/pkg/auth"
)

// setupGatewayTreeNoAuthMode writes a gateway tree like setupGatewayTree but
// with no auth_mode set (the real-world "registered but auth not yet
// configured" state), optionally with mtls/ca.crt present.
func setupGatewayTreeNoAuthMode(t *testing.T, withTLSMaterial bool) {
	t.Helper()
	root := t.TempDir()
	xdg := filepath.Join(root, "config")
	gwDir := filepath.Join(xdg, "openshell", "gateways", "rosa")
	if err := os.MkdirAll(gwDir, 0o700); err != nil {
		t.Fatal(err)
	}
	md := `{"name":"rosa","gateway_endpoint":"https://gw.example.com","is_remote":false,"gateway_port":443}`
	if err := os.WriteFile(filepath.Join(gwDir, "metadata.json"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "openshell", "active_gateway"), []byte("rosa"), 0o600); err != nil {
		t.Fatal(err)
	}
	if withTLSMaterial {
		mtlsDir := filepath.Join(gwDir, "mtls")
		if err := os.MkdirAll(mtlsDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(mtlsDir, "ca.crt"), []byte("fake-ca"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", xdg)
}

// TestResolveTokenSource_TLSPresentWiring confirms resolveTokenSource
// actually plumbs gatewayconfig.TLSMaterialFor's result into
// auth.ResolveInput.TLSPresent: a resolved gateway with auth_mode unset and
// real mtls/ca.crt material must NOT hit ErrNoCredentials (a legitimate,
// resolved mTLS gateway), while the same gateway with no TLS material at all
// must.
func TestResolveTokenSource_TLSPresentWiring(t *testing.T) {
	t.Run("mtls material present -> no ErrNoCredentials", func(t *testing.T) {
		setupGatewayTreeNoAuthMode(t, true)
		_, err := runCmd(t, "token", "show")
		if err != nil && errors.Is(err, auth.ErrNoCredentials) {
			t.Fatalf("expected no ErrNoCredentials with mtls material present, got: %v", err)
		}
	})

	t.Run("no tls material at all -> ErrNoCredentials", func(t *testing.T) {
		setupGatewayTreeNoAuthMode(t, false)
		_, err := runCmd(t, "token", "show")
		if !errors.Is(err, auth.ErrNoCredentials) {
			t.Fatalf("err = %v, want ErrNoCredentials", err)
		}
	})
}
