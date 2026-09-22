package cli

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "openshellctl-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	os.Setenv("HOME", tmp)
	os.Setenv("XDG_CONFIG_HOME", tmp)
	os.Setenv("OPENSHELL_TOKEN", "")
	os.Setenv("OPENSHELL_GATEWAY", "")
	os.Setenv("OPENSHELL_GATEWAY_ENDPOINT", "")
	os.Setenv("OPENSHELL_GATEWAY_INSECURE", "")
	os.Setenv("OPENSHELL_WORKSPACE", "")
	os.Setenv("OPENSHELL_OIDC_ISSUER", "")
	os.Setenv("OPENSHELL_OIDC_CLIENT_ID", "")
	os.Setenv("OPENSHELL_OIDC_CLIENT_SECRET", "")
	os.Setenv("OPENSHELL_OIDC_AUDIENCE", "")

	os.Exit(m.Run())
}
