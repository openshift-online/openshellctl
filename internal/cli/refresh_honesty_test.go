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
	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
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
	// The message now also lists what was checked (PR review: a bare "no
	// credentials configured" gives the user no idea where to look), so this
	// checks the stable prefix and the checked-paths detail separately
	// rather than the whole string verbatim.
	if !strings.HasPrefix(err.Error(), "nothing to refresh: no credentials configured") {
		t.Errorf("message = %q, want it to start with %q", err.Error(), "nothing to refresh: no credentials configured")
	}
	if !strings.Contains(err.Error(), "OPENSHELL_OIDC_CLIENT_SECRET") || !strings.Contains(err.Error(), "mtls/") {
		t.Errorf("message = %q, want it to list what was checked", err.Error())
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

// TestTokenRefresh_WriteWithUnknownGateway_SurfacesResolveError confirms
// --write's early requireTokenWriter check does NOT run for a resolve error
// other than ErrNoCredentials: `--write -g <typo'd name>` must surface the
// real problem (UnknownGatewayError, exit 4) rather than the generic writer
// usage error (exit 2), which would mask the typo and the actionable
// "list available gateways" remediation UnknownGatewayError already gives.
// This is the regression a PR review caught in the original "preempt
// unconditionally" implementation.
func TestTokenRefresh_WriteWithUnknownGateway_SurfacesResolveError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runCmd(t, "token", "refresh", "--write", "--gateway", "bogus")
	if err == nil {
		t.Fatal("expected an error")
	}
	var usage *UsageError
	if errors.As(err, &usage) {
		t.Fatalf("err = %v, want the resolve error (UnknownGatewayError), not the writer usage error", err)
	}
	if !errors.Is(err, gatewayconfig.ErrUnknownGateway) {
		t.Fatalf("err = %v, want UnknownGatewayError", err)
	}
	if exitCodeFor(err) != ExitNotFound {
		t.Errorf("exit = %d, want ExitNotFound", exitCodeFor(err))
	}
}

// TestTokenRefresh_WriteWithNothingConfigured_SurfacesNoActiveGateway
// confirms that with absolutely nothing registered and no flags at all,
// --write surfaces the pre-existing, already-correct NoActiveGatewayError
// (exit 4) rather than the writer usage error: per PR review, the early
// writer check must be scoped to ErrNoCredentials specifically, not to
// "any resolve error", so it never masks a more fundamental problem
// (unregistered gateway, typo'd name) with a less specific one.
func TestTokenRefresh_WriteWithNothingConfigured_SurfacesNoActiveGateway(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runCmd(t, "token", "refresh", "--write")
	if err == nil {
		t.Fatal("expected an error")
	}
	var usage *UsageError
	if errors.As(err, &usage) {
		t.Fatalf("err = %v, want NoActiveGatewayError, not the writer usage error", err)
	}
	if !errors.Is(err, gatewayconfig.ErrNoActiveGateway) {
		t.Fatalf("err = %v, want NoActiveGatewayError", err)
	}
	if exitCodeFor(err) != ExitNotFound {
		t.Errorf("exit = %d, want ExitNotFound", exitCodeFor(err))
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
