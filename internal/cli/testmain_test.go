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

	for k, v := range map[string]string{"HOME": tmp, "XDG_CONFIG_HOME": tmp} {
		if err := os.Setenv(k, v); err != nil {
			panic(err)
		}
	}
	for _, k := range []string{
		"OPENSHELL_TOKEN", "OPENSHELL_GATEWAY", "OPENSHELL_GATEWAY_ENDPOINT",
		"OPENSHELL_GATEWAY_INSECURE", "OPENSHELL_WORKSPACE",
		"OPENSHELL_OIDC_ISSUER", "OPENSHELL_OIDC_CLIENT_ID",
		"OPENSHELL_OIDC_CLIENT_SECRET", "OPENSHELL_OIDC_AUDIENCE",
	} {
		if err := os.Unsetenv(k); err != nil {
			panic(err)
		}
	}

	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}
