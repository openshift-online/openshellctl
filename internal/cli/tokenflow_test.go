package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// setupGatewayTree writes a minimal user config tree with one oidc gateway and
// points XDG_CONFIG_HOME at it. Returns the config root.
func setupGatewayTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	xdg := filepath.Join(root, "config")
	gwDir := filepath.Join(xdg, "openshell", "gateways", "rosa")
	if err := os.MkdirAll(gwDir, 0o700); err != nil {
		t.Fatal(err)
	}
	md := `{"name":"rosa","gateway_endpoint":"https://gw.example.com","is_remote":false,"gateway_port":443,"auth_mode":"oidc","oidc_issuer":"https://issuer"}`
	if err := os.WriteFile(filepath.Join(gwDir, "metadata.json"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "openshell", "active_gateway"), []byte("rosa"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
	return xdg
}

func staticJWT(t *testing.T) string {
	t.Helper()
	enc := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	payload := map[string]any{
		"iss":          "https://issuer",
		"sub":          "user-9",
		"aud":          "openshell-cli",
		"exp":          float64(time.Now().Add(time.Hour).Unix()),
		"realm_access": map[string]any{"roles": []any{"openshell-user"}},
	}
	return enc(map[string]any{"alg": "RS256"}) + "." + enc(payload) + ".c2ln"
}

// TestTokenShow_StaticToken exercises resolveTokenSource end-to-end with a
// static token against a temp gateway tree (no network for token resolution;
// the whoami dial is best-effort and its failure is a warning).
func TestTokenShow_StaticToken(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	setupGatewayTree(t)
	t.Setenv("OPENSHELL_TOKEN", staticJWT(t))

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"token", "show"})
	if err := root.Execute(); err != nil {
		t.Fatalf("token show: %v", err)
	}
	got := out.String()
	for _, want := range []string{"user-9", "openshell-cli", "openshell-user"} {
		if !strings.Contains(got, want) {
			t.Errorf("token show output missing %q; got:\n%s", want, got)
		}
	}
}

func TestTokenShow_JSON(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	setupGatewayTree(t)
	t.Setenv("OPENSHELL_TOKEN", staticJWT(t))

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"token", "show", "-o", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("token show -o json: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out.String())
	}
	if parsed["subject"] != "user-9" {
		t.Errorf("subject = %v", parsed["subject"])
	}
}

func TestExecute_UnknownCommandUsageExit(t *testing.T) {
	// Execute() builds its own root; an unknown flag must map to exit 2.
	os.Args = []string{"openshellctl", "--nonexistent"}
	code := Execute()
	if code != ExitUsage {
		t.Errorf("Execute() = %d, want %d (usage)", code, ExitUsage)
	}
}

func TestExecute_Success(t *testing.T) {
	os.Args = []string{"openshellctl", "version"}
	if code := Execute(); code != ExitOK {
		t.Errorf("Execute(version) = %d, want 0", code)
	}
}

func TestUsageError_Message(t *testing.T) {
	e := &UsageError{Err: os.ErrInvalid}
	if e.Error() == "" {
		t.Error("UsageError.Error() should be non-empty")
	}
	if e.Unwrap() != os.ErrInvalid {
		t.Error("UsageError should unwrap to its cause")
	}
}
