package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestGatewayCommandTree(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	root := NewRootCommand()
	names := collectCommandNames(root)

	g, ok := names["gateway"]
	if !ok {
		t.Fatal("gateway command missing")
	}
	foundAlias := false
	for _, a := range g.Aliases {
		if a == "gw" {
			foundAlias = true
		}
	}
	if !foundAlias {
		t.Errorf("gateway is missing the 'gw' alias, got %v", g.Aliases)
	}
	children := map[string]bool{}
	for _, c := range g.Commands() {
		children[c.Name()] = true
	}
	for _, want := range []string{"add", "list", "select", "remove", "logout", "login"} {
		if !children[want] {
			t.Errorf("gateway is missing child %q", want)
		}
	}
}

func TestGatewayList_Empty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	out, err := runCmd(t, "gateway", "list")
	if err != nil {
		t.Fatalf("gateway list: %v", err)
	}
	if !strings.Contains(out, "No gateways found.") {
		t.Errorf("out = %q", out)
	}
}

func TestGatewayList_ShowsActiveMarkerAndAuth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENSHELL_NO_BROWSER", "1")
	if _, err := runCmd(t, "gateway", "add", "https://gw.example.com", "--name", "test-gw", "--oidc-issuer", "https://issuer"); err != nil {
		t.Fatalf("setup add: %v", err)
	}

	out, err := runCmd(t, "gateway", "list")
	if err != nil {
		t.Fatalf("gateway list: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if !strings.HasPrefix(lines[0], "NAME") || !strings.Contains(lines[0], "AUTH") {
		t.Errorf("header = %q", lines[0])
	}
	var row string
	for _, l := range lines[1:] {
		if strings.Contains(l, "test-gw") {
			row = l
		}
	}
	if !strings.HasPrefix(row, "*") {
		t.Errorf("active gateway row should be marked with *, got: %q", row)
	}
	if !strings.Contains(row, "oidc") {
		t.Errorf("row missing AUTH=oidc: %q", row)
	}
}

func TestGatewayList_JSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENSHELL_NO_BROWSER", "1")
	if _, err := runCmd(t, "gateway", "add", "https://gw.example.com", "--name", "test-gw", "--oidc-issuer", "https://issuer"); err != nil {
		t.Fatalf("setup add: %v", err)
	}
	out, err := runCmd(t, "gateway", "list", "-o", "json")
	if err != nil {
		t.Fatalf("gateway list -o json: %v", err)
	}
	if !strings.Contains(out, `"name": "test-gw"`) && !strings.Contains(out, `"name":"test-gw"`) {
		t.Errorf("json output missing name: %s", out)
	}
}

func TestGatewaySelect_SetsActive(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENSHELL_NO_BROWSER", "1")
	for _, name := range []string{"a", "b"} {
		if _, err := runCmd(t, "gateway", "add", "https://"+name+".example.com", "--name", name, "--oidc-issuer", "https://issuer"); err != nil {
			t.Fatalf("setup add %s: %v", name, err)
		}
	}
	// "b" was added last and so is active; select "a" and confirm it becomes active.
	out, err := runCmd(t, "gateway", "select", "a")
	if err != nil {
		t.Fatalf("gateway select: %v", err)
	}
	if !strings.Contains(out, "✓ Active gateway set to 'a'") {
		t.Errorf("confirmation message mismatch (upstream gateway.rs:485): %q", out)
	}
	listOut, err := runCmd(t, "gateway", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(listOut, "\n") {
		if strings.Contains(l, " a ") || strings.HasSuffix(strings.TrimRight(l, " "), "a") {
			if !strings.HasPrefix(l, "*") {
				t.Errorf("gateway a should be active after select: %q", l)
			}
		}
	}
}

// TestGatewaySelect_EnvOverrideWarning pins ROSAENG-74241 item 2 end to end:
// selecting a gateway while OPENSHELL_GATEWAY names a different one prints
// the upstream warning (gateway.rs:490-499) after the success line.
func TestGatewaySelect_EnvOverrideWarning(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENSHELL_NO_BROWSER", "1")
	if _, err := runCmd(t, "gateway", "add", "https://a.example.com", "--name", "a", "--oidc-issuer", "https://issuer"); err != nil {
		t.Fatalf("setup add: %v", err)
	}
	t.Setenv("OPENSHELL_GATEWAY", "b")

	out, err := runCmd(t, "gateway", "select", "a")
	if err != nil {
		t.Fatalf("gateway select: %v", err)
	}
	want := "OPENSHELL_GATEWAY=b is set and will override this selection.\n  Unset it or run: export OPENSHELL_GATEWAY=a"
	if !strings.Contains(out, want) {
		t.Errorf("env-override warning missing or wrong, got: %q", out)
	}
}

func TestGatewaySelect_UnknownGateway(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runCmd(t, "gateway", "select", "ghost")
	if err == nil {
		t.Fatal("expected an error selecting an unregistered gateway")
	}
	if exitCodeFor(err) != ExitNotFound {
		t.Errorf("exit = %d, want not-found", exitCodeFor(err))
	}
}

func TestGatewayRemove_RemovesRegistration(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENSHELL_NO_BROWSER", "1")
	if _, err := runCmd(t, "gateway", "add", "https://gw.example.com", "--name", "test-gw", "--oidc-issuer", "https://issuer"); err != nil {
		t.Fatalf("setup add: %v", err)
	}
	removeOut, err := runCmd(t, "gateway", "remove", "test-gw")
	if err != nil {
		t.Fatalf("gateway remove: %v", err)
	}
	if !strings.Contains(removeOut, "✓ Gateway registration 'test-gw' removed.") {
		t.Errorf("confirmation message mismatch (upstream gateway.rs:1498-1501): %q", removeOut)
	}
	out, err := runCmd(t, "gateway", "list")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "test-gw") {
		t.Errorf("removed gateway should not appear in list: %s", out)
	}
}

func TestGatewayRemove_NotFound(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runCmd(t, "gateway", "remove", "ghost")
	if err == nil {
		t.Fatal("expected an error removing an unregistered gateway")
	}
	if exitCodeFor(err) != ExitNotFound {
		t.Errorf("exit = %d, want not-found", exitCodeFor(err))
	}
}

func TestGatewayLogout_RemovesTokenKeepsRegistration(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENSHELL_NO_BROWSER", "1")
	if _, err := runCmd(t, "gateway", "add", "https://gw.example.com", "--name", "test-gw", "--oidc-issuer", "https://issuer"); err != nil {
		t.Fatalf("setup add: %v", err)
	}
	if _, err := runCmd(t, "gateway", "logout"); err != nil {
		t.Fatalf("gateway logout: %v", err)
	}
	// Registration (and active status) should survive logout.
	out, err := runCmd(t, "gateway", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "test-gw") {
		t.Errorf("gateway should still be registered after logout: %s", out)
	}
}

func TestGatewayLogout_NoActiveGateway(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runCmd(t, "gateway", "logout")
	if err == nil {
		t.Fatal("expected an error logging out with no active/named gateway")
	}
	if exitCodeFor(err) != ExitNotFound {
		t.Errorf("exit = %d, want not-found", exitCodeFor(err))
	}
}

// TestGatewayLogout_UnmatchedEndpointDoesNotFalselySucceed confirms logout
// with a --gateway-endpoint that doesn't match any registered gateway fails
// with a clear not-found error, rather than silently "succeeding": without
// the Load check in newGatewayLogoutCommand, gatewayconfig.Resolve's
// endpoint-only fallback sets target.Name to the raw endpoint string when no
// registered gateway matches it, and — for an endpoint value that happens to
// also be a valid single path component (no slash) — Logout's underlying
// Writer.Remove on a never-written path is a silent no-op, reporting success
// for a gateway that was never registered.
func TestGatewayLogout_UnmatchedEndpointDoesNotFalselySucceed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runCmd(t, "gateway", "logout", "--gateway-endpoint", "never-registered-host")
	if err == nil {
		t.Fatal("expected an error logging out of an endpoint that matches no registered gateway")
	}
	if exitCodeFor(err) != ExitNotFound {
		t.Errorf("exit = %d, want not-found, got err: %v", exitCodeFor(err), err)
	}
}

func TestGatewayLogin_RequiresGateway(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runCmd(t, "gateway", "login")
	if err == nil {
		t.Fatal("expected an error without a gateway")
	}
}

// TestGatewayLogout_UsesSlug confirms that when a gateway's directory slug
// (Resolved.Name) differs from its display name (Metadata.Name / Target.Name),
// logout uses the slug for filesystem operations and keeps the display name in
// the user-facing message.
func TestGatewayLogout_UsesSlug(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	// Build a gateway whose directory slug ("rosa-agentic-devx") differs from
	// its metadata display name ("ROSA Agentic Devx"), simulating a gateway
	// registered by the upstream Rust CLI that allows spaces in names.
	const slug = "rosa-agentic-devx"
	const displayName = "ROSA Agentic Devx"
	gwDir := filepath.Join(xdg, "openshell", "gateways", slug)
	if err := os.MkdirAll(gwDir, 0o700); err != nil {
		t.Fatal(err)
	}
	mdJSON := `{"name":"` + displayName + `","gateway_endpoint":"https://gw.example.com","is_remote":false,"gateway_port":443}`
	if err := os.WriteFile(filepath.Join(gwDir, "metadata.json"), []byte(mdJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	// Write a fake token so logout has something to remove.
	if err := os.WriteFile(filepath.Join(gwDir, "oidc_token.json"), []byte(`{"access_token":"tok"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "openshell", "active_gateway"), []byte(slug), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "gateway", "logout")
	if err != nil {
		t.Fatalf("gateway logout: %v\noutput: %s", err, out)
	}
	// Token removed.
	if _, statErr := os.Stat(filepath.Join(gwDir, "oidc_token.json")); statErr == nil {
		t.Error("expected oidc_token.json to be removed after logout")
	}
	// User-facing message uses display name (upstream parity).
	if !strings.Contains(out, displayName) {
		t.Errorf("logout output should include display name %q, got: %s", displayName, out)
	}
}
