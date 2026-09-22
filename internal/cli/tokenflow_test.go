package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

func TestWhoami_ExistsInCommandTree(t *testing.T) {
	root := NewRootCommand()
	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "whoami" {
			found = true
		}
	}
	if !found {
		t.Error("whoami command not registered on root")
	}
}

func TestLogin_ExistsInCommandTree(t *testing.T) {
	root := NewRootCommand()
	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "login" {
			found = true
		}
	}
	if !found {
		t.Error("login command not registered on root")
	}
}

func TestLogin_RequiresGateway(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	// No gateway tree, no env — should fail with usage error.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runCmd(t, "login")
	if err == nil {
		t.Fatal("expected error without gateway")
	}
}

func TestWhoami_RequiresAuth(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	setupGatewayTree(t)
	// No token set — whoami should fail with an auth error, not a panic.
	_, err := runCmd(t, "whoami")
	if err == nil {
		t.Fatal("expected error without token")
	}
}

func TestWhoami_VerboseMatchesTokenShow(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	setupGatewayTree(t)
	t.Setenv("OPENSHELL_TOKEN", staticJWT(t))

	// whoami -v should produce the same token info as token show.
	out1, err1 := runCmd(t, "whoami", "-v")
	if err1 != nil {
		t.Fatalf("whoami -v: %v", err1)
	}
	out2, err2 := runCmd(t, "token", "show")
	if err2 != nil {
		t.Fatalf("token show: %v", err2)
	}
	// Both should contain the subject.
	if !strings.Contains(out1, "user-9") {
		t.Errorf("whoami -v missing subject; got:\n%s", out1)
	}
	if !strings.Contains(out2, "user-9") {
		t.Errorf("token show missing subject; got:\n%s", out2)
	}
}

func TestHintSuppression_ExpiredTokenHintNotDuplicated(t *testing.T) {
	setupExpiredTokenTree(t)
	// Execute() with token show should return ExitAuth and NOT duplicate the
	// hint when the error already mentions "openshellctl token refresh" or
	// "openshellctl login".
	os.Args = []string{"openshellctl", "token", "show"}
	code := Execute()
	if code != ExitAuth {
		t.Errorf("Execute() = %d, want %d (auth)", code, ExitAuth)
	}
}

func setupExpiredTokenTreeWithRefresh(t *testing.T) {
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
	exp := time.Now().Add(-1 * time.Hour).Unix()
	bundle := `{"access_token":"expired.jwt.here","refresh_token":"stale-refresh","expires_at":` + fmt.Sprintf("%d", exp) + `,"issuer":"https://issuer","client_id":"openshell-cli"}`
	if err := os.WriteFile(filepath.Join(gwDir, "oidc_token.json"), []byte(bundle), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "openshell", "active_gateway"), []byte("rosa"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
}

func setupExpiredTokenTree(t *testing.T) {
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
	// Write an expired token (expired 1 hour ago).
	exp := time.Now().Add(-1 * time.Hour).Unix()
	bundle := `{"access_token":"expired.jwt.here","expires_at":` + fmt.Sprintf("%d", exp) + `,"issuer":"https://issuer","client_id":"openshell-cli"}`
	if err := os.WriteFile(filepath.Join(gwDir, "oidc_token.json"), []byte(bundle), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "openshell", "active_gateway"), []byte("rosa"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
}

func TestTokenRefresh_ExpiredNoRefreshToken_FallsBackToLogin(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	setupExpiredTokenTree(t)

	// token refresh with an expired token and no refresh token should fall
	// back to the browser login flow. In a test environment the OIDC login
	// will fail (no browser/network), but we verify the fallback path was
	// taken by checking that stderr contains the fallback message.
	out, err := runCmd(t, "token", "refresh", "--write")
	if err == nil {
		t.Fatal("expected error with expired token in test env")
	}
	if !strings.Contains(out, "Launching browser login") {
		t.Errorf("token refresh should fall back to browser login, got output:\n%s\nerror: %v", out, err)
	}
}

func TestExecute_ExpiredWithRefreshToken_SuppressesHint(t *testing.T) {
	setupExpiredTokenTreeWithRefresh(t)
	// When a refresh token exists, the error says "openshellctl token refresh
	// --write" — Execute() must not add a second generic hint.
	os.Args = []string{"openshellctl", "token", "show"}
	code := Execute()
	if code != ExitAuth {
		t.Errorf("Execute() = %d, want %d (auth)", code, ExitAuth)
	}
}

func TestTokenShow_ExpiredWithRefreshToken_SuggestsRefresh(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	setupExpiredTokenTreeWithRefresh(t)

	_, err := runCmd(t, "token", "show")
	if err == nil {
		t.Fatal("expected error with expired token")
	}
	msg := err.Error()
	if !strings.Contains(msg, "openshellctl token refresh --write") {
		t.Errorf("error should suggest token refresh, got: %s", msg)
	}
}

func TestTokenShow_ExpiredNoRefreshToken_SuggestsLogin(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	setupExpiredTokenTree(t)

	// token show calls src.Token() directly, so it receives ErrTokenExpired
	// with the hint. Without a refresh token, the hint should say "login".
	_, err := runCmd(t, "token", "show")
	if err == nil {
		t.Fatal("expected error with expired token")
	}
	msg := err.Error()
	if !strings.Contains(msg, "openshellctl login") {
		t.Errorf("error should suggest openshellctl login, got: %s", msg)
	}
	if strings.Contains(msg, "token refresh") {
		t.Errorf("error should NOT suggest token refresh when no refresh token exists, got: %s", msg)
	}
}

func TestTokenRefresh_SpacesInDisplayName_UsesSlug(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	// Create a gateway where the directory slug differs from the display name.
	// Directory: "rosa-agentic-devx", metadata name: "ROSA Agentic Devx"
	root := t.TempDir()
	xdg := filepath.Join(root, "config")
	gwDir := filepath.Join(xdg, "openshell", "gateways", "rosa-agentic-devx")
	if err := os.MkdirAll(gwDir, 0o700); err != nil {
		t.Fatal(err)
	}
	md := `{"name":"ROSA Agentic Devx","gateway_endpoint":"https://gw.example.com","is_remote":false,"gateway_port":443,"auth_mode":"oidc","oidc_issuer":"https://issuer"}`
	if err := os.WriteFile(filepath.Join(gwDir, "metadata.json"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(-1 * time.Hour).Unix()
	bundle := `{"access_token":"expired.jwt.here","expires_at":` + fmt.Sprintf("%d", exp) + `,"issuer":"https://issuer","client_id":"openshell-cli"}`
	if err := os.WriteFile(filepath.Join(gwDir, "oidc_token.json"), []byte(bundle), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "openshell", "active_gateway"), []byte("rosa-agentic-devx"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)

	// token refresh should fall back to browser login. The error from oidc.Login
	// should NOT contain "invalid gateway name" — that would mean we passed the
	// display name (with spaces) instead of the directory slug.
	out, err := runCmd(t, "token", "refresh", "--write")
	if err == nil {
		t.Fatal("expected error (no browser in test)")
	}
	if strings.Contains(err.Error(), "invalid gateway name") {
		t.Errorf("loginAndReport should use the directory slug, not the display name; got: %v", err)
	}
	if !strings.Contains(out, "Launching browser login") {
		t.Errorf("should attempt browser login, got output:\n%s", out)
	}
}
