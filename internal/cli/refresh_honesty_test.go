package cli

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"go.uber.org/mock/gomock"

	"github.com/openshift-online/openshellctl/pkg/auth"
	"github.com/openshift-online/openshellctl/pkg/gateway/mock"
)

// TestTokenRefresh_NoCredentials_NothingToRefresh confirms the actual
// reproduction of the fake-success bug: an unmatched --gateway-endpoint (no
// local metadata, no name match) with no secret reaches auth.Resolve with
// Gateway == nil, which now returns ErrNoCredentials instead of a
// working-looking NoAuthSource. token refresh wraps it as
// ErrNothingToRefresh, producing the exact message/exit code the ticket's
// acceptance criteria ask for.
//
// Note: the ticket's own literal first "How to Verify" command (`token
// refresh` with zero flags and nothing registered) does NOT reach this code
// path — gatewayconfig.Resolve's existing, already-correct
// NoActiveGatewayError short-circuits before auth.Resolve is ever called,
// which is left untouched (documented in the PR).
func TestTokenRefresh_NoCredentials_NothingToRefresh(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	out, err := runCmd(t, "token", "refresh", "--gateway-endpoint", "https://nowhere.invalid")
	if err == nil {
		t.Fatalf("expected an error, got success:\n%s", out)
	}
	var nothingToRefresh *auth.ErrNothingToRefresh
	if !errors.As(err, &nothingToRefresh) {
		t.Fatalf("err = %v, want ErrNothingToRefresh", err)
	}
	if err.Error() != "nothing to refresh: no credentials configured" {
		t.Errorf("message = %q, want exactly %q", err.Error(), "nothing to refresh: no credentials configured")
	}
	// exitCodeFor's mapping for ErrNothingToRefresh is added in a later
	// commit (exit-code completion); see exitcode_test.go for that assertion.
	if strings.Contains(out, `refreshed token for subject ""`) {
		t.Errorf("must not print the old fake-success message, got:\n%s", out)
	}
}

// TestTokenRefresh_WriteWithUnresolvedGateway_UsageError confirms --write's
// early requireTokenWriter check takes priority over the ErrNoCredentials
// path when both would otherwise apply — "nowhere to write to" is the more
// specific, actionable problem. Matches the ticket's second verify command.
func TestTokenRefresh_WriteWithUnresolvedGateway_UsageError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runCmd(t, "token", "refresh", "--write", "--gateway-endpoint", "https://nowhere.invalid")
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
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want ExitUsage", exitCodeFor(err))
	}
}

// TestTokenRefresh_WriteWithNothingConfigured_UsageError confirms --write's
// early requireTokenWriter check takes priority even when resolveAuth itself
// fails with the (unrelated, pre-existing, correct) NoActiveGatewayError —
// i.e. absolutely nothing is registered and no --gateway*/flag was given at
// all. "Nowhere to write to" is still the more specific, actionable problem,
// so this intentionally changes the exit code for this exact invocation from
// 4 (NoActiveGatewayError) to 2 (UsageError) — a deliberate, not merely
// incidental, consequence of the ordering in newTokenRefreshCommand.
func TestTokenRefresh_WriteWithNothingConfigured_UsageError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runCmd(t, "token", "refresh", "--write")
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
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want ExitUsage", exitCodeFor(err))
	}
}

// TestTokenRefresh_SourceNone_NothingToRefresh confirms a resolved gateway
// with an explicit no-auth mode (auth_mode: none — a legitimate state, not a
// misconfiguration) also rejects refresh instead of fake-succeeding: there
// is no openshellctl-managed bearer token for a no-auth gateway, so "nothing
// to refresh" is the honest answer regardless of whether Resolve itself
// errored.
func TestTokenRefresh_SourceNone_NothingToRefresh(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "config")
	gwDir := filepath.Join(xdg, "openshell", "gateways", "rosa")
	if err := os.MkdirAll(gwDir, 0o700); err != nil {
		t.Fatal(err)
	}
	md := `{"name":"rosa","gateway_endpoint":"https://gw.example.com","is_remote":false,"gateway_port":443,"auth_mode":"none"}`
	if err := os.WriteFile(filepath.Join(gwDir, "metadata.json"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "openshell", "active_gateway"), []byte("rosa"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	_, err := runCmd(t, "token", "refresh")
	if err == nil {
		t.Fatal("expected an error for a no-auth gateway")
	}
	var nothingToRefresh *auth.ErrNothingToRefresh
	if !errors.As(err, &nothingToRefresh) {
		t.Fatalf("err = %v, want ErrNothingToRefresh", err)
	}
}

// staticJWTWithAudience builds a minimal unsigned JWT (matching tokenflow_test.go's
// staticJWT helper) with a caller-chosen aud claim.
func staticJWTWithAudience(t *testing.T, aud string) string {
	t.Helper()
	enc := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	payload := map[string]any{
		"iss":          "https://issuer",
		"sub":          "user-9",
		"aud":          aud,
		"exp":          float64(time.Now().Add(time.Hour).Unix()),
		"realm_access": map[string]any{"roles": []any{"openshell-user"}},
	}
	return enc(map[string]any{"alg": "RS256"}) + "." + enc(payload) + ".c2ln"
}

// TestTokenShow_PreflightWarnings_AudienceMismatch confirms token show
// prints a PreflightWarnings warning to stderr for a static token whose
// audience doesn't match --oidc-audience. A mock gateway is injected via the
// Feature 0 cliDeps seam (same pattern as TestTokenShow_StaticToken) so
// reportCurrentUser's whoami dial hits it instead of attempting a real
// network connection.
func TestTokenShow_PreflightWarnings_AudienceMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().CurrentUser(gomock.Any()).Return(&types.CurrentUser{Subject: "user-9"}, nil)

	jwt := staticJWTWithAudience(t, "some-other-audience")
	deps := cliDeps{Gateway: gw, TokenSource: auth.NewStaticSource(jwt, nil)}
	out, err := runCmdWithGateway(t, deps, "token", "show", "--oidc-audience", "openshell-cli")
	if err != nil {
		t.Fatalf("token show: %v", err)
	}
	if !strings.Contains(out, "does not include the expected audience") {
		t.Errorf("expected an audience-mismatch warning, got:\n%s", out)
	}
}

// TestTokenShow_PreflightWarnings_SkippedForJSON confirms the warning is NOT
// printed in -o json mode, matching reportCurrentUser's existing convention
// of keeping JSON output on stdout machine-parseable (this test harness
// merges stdout+stderr into one buffer, same as real `cmd 2>&1`, so any
// stderr text printed unconditionally would corrupt the JSON here).
func TestTokenShow_PreflightWarnings_SkippedForJSON(t *testing.T) {
	jwt := staticJWTWithAudience(t, "some-other-audience")
	deps := cliDeps{TokenSource: auth.NewStaticSource(jwt, nil)}
	out, err := runCmdWithGateway(t, deps, "token", "show", "-o", "json", "--oidc-audience", "openshell-cli")
	if err != nil {
		t.Fatalf("token show: %v", err)
	}
	if strings.Contains(out, "does not include the expected audience") {
		t.Errorf("warning should not be printed in json mode, got:\n%s", out)
	}
}
