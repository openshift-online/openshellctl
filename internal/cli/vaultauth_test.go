package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/viper"

	"github.com/openshift-online/openshellctl/pkg/vaultconfig"
)

// fakeVaultReader is a hermetic VaultReader double — no network, no real
// Vault server — used to drive applyVaultAuthSource's wiring logic.
type fakeVaultReader struct {
	data  map[string]any
	err   error
	calls int
}

func (f *fakeVaultReader) ReadSecret(_ context.Context, _, _ string) (map[string]any, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.data, nil
}

func withFakeVaultReader(reader vaultconfig.VaultReader) context.Context {
	return withVaultDeps(context.Background(), vaultDeps{Reader: reader})
}

// resetVaultOnce resets applyVaultAuthSource's process-lifetime memoization
// (vaultAuthOnce/vaultAuthResult). Shared by resetVaultTestState here and by
// runCmdCtx (sandbox_cmd_test.go), so every test path that can reach
// applyVaultAuthSource resets the same two package vars the same way.
func resetVaultOnce() {
	vaultAuthOnce = sync.Once{}
	vaultAuthResult = nil
}

// resetVaultTestState resets both viper and the Vault-auth memoization,
// before and after the test. Without the latter, the first test in the
// package to call applyVaultAuthSource would permanently cache its result for
// every test that runs afterward in the same test binary.
func resetVaultTestState(t *testing.T) {
	t.Helper()
	reset := func() {
		viper.Reset()
		resetVaultOnce()
	}
	reset()
	t.Cleanup(reset)
}

func TestApplyVaultAuthSource_NoopWhenNeitherFlagSet(t *testing.T) {
	resetVaultTestState(t)

	reader := &fakeVaultReader{data: map[string]any{"oidc-issuer": "https://issuer"}}
	if err := applyVaultAuthSource(withFakeVaultReader(reader)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reader.calls != 0 {
		t.Errorf("reader was called %d times, want 0", reader.calls)
	}
	if got := viper.GetString("oidc-issuer"); got != "" {
		t.Errorf("oidc-issuer = %q, want empty (no-op)", got)
	}
}

func TestApplyVaultAuthSource_UsageErrorWhenOnlyMountSet(t *testing.T) {
	resetVaultTestState(t)
	viper.Set("vault-kv-mount", "osd-sre")

	err := applyVaultAuthSource(withFakeVaultReader(&fakeVaultReader{}))
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *UsageError, got %T: %v", err, err)
	}
	for _, want := range []string{"--vault-kv-mount", "--vault-kv-path"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message %q missing %q", err.Error(), want)
		}
	}
}

func TestApplyVaultAuthSource_UsageErrorWhenOnlyPathSet(t *testing.T) {
	resetVaultTestState(t)
	viper.Set("vault-kv-path", "rosa-agent")

	err := applyVaultAuthSource(withFakeVaultReader(&fakeVaultReader{}))
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *UsageError, got %T: %v", err, err)
	}
}

// TestApplyVaultAuthSource_UsageErrorWhenOnlyPrefixSet pins a specific
// misconfiguration: --vault-field-prefix has no effect without
// --vault-kv-mount/--vault-kv-path, and silently ignoring it (rather than
// erroring) would mask a typo'd or forgotten mount/path flag.
func TestApplyVaultAuthSource_UsageErrorWhenOnlyPrefixSet(t *testing.T) {
	resetVaultTestState(t)
	viper.Set("vault-field-prefix", "hypershell")

	err := applyVaultAuthSource(withFakeVaultReader(&fakeVaultReader{}))
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *UsageError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "--vault-field-prefix") {
		t.Errorf("error message %q should name --vault-field-prefix", err.Error())
	}
}

func TestApplyVaultAuthSource_FillsDefaultsFromPrefixedFields(t *testing.T) {
	resetVaultTestState(t)
	viper.Set("vault-kv-mount", "osd-sre")
	viper.Set("vault-kv-path", "rosa-agent")
	viper.Set("vault-field-prefix", "hypershell")

	reader := &fakeVaultReader{data: map[string]any{
		"hypershell-oidc-client-id":     "abc",
		"hypershell-oidc-client-secret": "shh",
	}}
	if err := applyVaultAuthSource(withFakeVaultReader(reader)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reader.calls != 1 {
		t.Errorf("reader was called %d times, want 1", reader.calls)
	}
	if got := viper.GetString("oidc-client-id"); got != "abc" {
		t.Errorf("oidc-client-id = %q, want %q", got, "abc")
	}
	if got := viper.GetString("oidc.client_secret"); got != "shh" {
		t.Errorf("oidc.client_secret = %q, want %q", got, "shh")
	}
}

func TestApplyVaultAuthSource_ExplicitGatewayFlagBeatsVaultForBothHalvesOfThePair(t *testing.T) {
	resetVaultTestState(t)
	viper.Set("vault-kv-mount", "osd-sre")
	viper.Set("vault-kv-path", "rosa-agent")
	viper.Set("gateway", "mine") // simulates an explicit --gateway/env

	reader := &fakeVaultReader{data: map[string]any{
		"gateway":          "vault-gw",
		"gateway-endpoint": "https://vault.example.com",
	}}
	if err := applyVaultAuthSource(withFakeVaultReader(reader)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := viper.GetString("gateway"); got != "mine" {
		t.Errorf("gateway = %q, want %q (explicit value must win)", got, "mine")
	}
	if got := viper.GetString("gateway-endpoint"); got != "" {
		t.Errorf("gateway-endpoint = %q, want empty (vault's half of the pair must be suppressed too)", got)
	}
}

func TestApplyVaultAuthSource_ClientSecretFileBeatsVaultSecret(t *testing.T) {
	resetVaultTestState(t)
	viper.Set("vault-kv-mount", "osd-sre")
	viper.Set("vault-kv-path", "rosa-agent")
	viper.Set("client-secret-file", "/tmp/my-secret")

	reader := &fakeVaultReader{data: map[string]any{
		"oidc-client-id":     "vault-client",
		"oidc-client-secret": "vault-secret",
	}}
	if err := applyVaultAuthSource(withFakeVaultReader(reader)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := viper.GetString("oidc.client_secret"); got != "" {
		t.Errorf("oidc.client_secret = %q, want empty (--client-secret-file must win)", got)
	}
	if got := viper.GetString("oidc-client-id"); got != "" {
		t.Errorf("oidc-client-id = %q, want empty (client-secret-file occupies the whole pair)", got)
	}
}

func TestApplyVaultAuthSource_UnrelatedFieldsStillFillWhenPairIsOverridden(t *testing.T) {
	resetVaultTestState(t)
	viper.Set("vault-kv-mount", "osd-sre")
	viper.Set("vault-kv-path", "rosa-agent")
	viper.Set("gateway", "mine")

	reader := &fakeVaultReader{data: map[string]any{
		"gateway":       "vault-gw",
		"oidc-issuer":   "https://issuer",
		"oidc-audience": "openshell-cli",
	}}
	if err := applyVaultAuthSource(withFakeVaultReader(reader)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := viper.GetString("oidc-issuer"); got != "https://issuer" {
		t.Errorf("oidc-issuer = %q, want %q", got, "https://issuer")
	}
	if got := viper.GetString("oidc-audience"); got != "openshell-cli" {
		t.Errorf("oidc-audience = %q, want %q", got, "openshell-cli")
	}
}

func TestApplyVaultAuthSource_ReaderErrorPropagates(t *testing.T) {
	resetVaultTestState(t)
	viper.Set("vault-kv-mount", "osd-sre")
	viper.Set("vault-kv-path", "rosa-agent")

	wantErr := &vaultconfig.ErrForbidden{Mount: "osd-sre", Path: "rosa-agent"}
	reader := &fakeVaultReader{err: wantErr}
	err := applyVaultAuthSource(withFakeVaultReader(reader))
	var forbidden *vaultconfig.ErrForbidden
	if !errors.As(err, &forbidden) {
		t.Fatalf("expected *vaultconfig.ErrForbidden, got %T: %v", err, err)
	}
}

func TestApplyVaultAuthSource_NonStringFieldErrors(t *testing.T) {
	resetVaultTestState(t)
	viper.Set("vault-kv-mount", "osd-sre")
	viper.Set("vault-kv-path", "rosa-agent")

	reader := &fakeVaultReader{data: map[string]any{"oidc-client-id": 123}}
	err := applyVaultAuthSource(withFakeVaultReader(reader))
	var fieldErr *vaultconfig.ErrFieldNotString
	if !errors.As(err, &fieldErr) {
		t.Fatalf("expected *vaultconfig.ErrFieldNotString, got %T: %v", err, err)
	}
}

func TestApplyVaultAuthSource_NoInjectedReaderBuildsRealOneAndFailsFastWithoutVaultAddr(t *testing.T) {
	resetVaultTestState(t)
	viper.Set("vault-kv-mount", "osd-sre")
	viper.Set("vault-kv-path", "rosa-agent")
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "")

	// No vaultDeps injected into the context: exercises the real,
	// production vaultReaderFor/realVaultReader path. With VAULT_ADDR
	// unset, this must fail fast without attempting any I/O.
	err := applyVaultAuthSource(context.Background())
	var addrErr *vaultconfig.ErrVaultAddrNotSet
	if !errors.As(err, &addrErr) {
		t.Fatalf("expected *vaultconfig.ErrVaultAddrNotSet, got %T: %v", err, err)
	}
}

// TestApplyVaultAuthSource_MemoizedAcrossMultipleCallsInOneProcess pins the
// fix for the double-read bug found in review: a single command whose flow
// reaches more than one call site (e.g. `gateway add` with a Vault-sourced
// client secret calls applyVaultAuthSource directly, then again via
// authenticateNewGateway -> resolveAuth -> resolveTokenSource) must only
// issue one Vault read per process, not one per call site — a single-use or
// response-wrapped Vault token would fail the second read outright.
func TestApplyVaultAuthSource_MemoizedAcrossMultipleCallsInOneProcess(t *testing.T) {
	resetVaultTestState(t)
	viper.Set("vault-kv-mount", "osd-sre")
	viper.Set("vault-kv-path", "rosa-agent")

	reader := &fakeVaultReader{data: map[string]any{"oidc-client-id": "abc"}}
	ctx := withFakeVaultReader(reader)

	if err := applyVaultAuthSource(ctx); err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if err := applyVaultAuthSource(ctx); err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if reader.calls != 1 {
		t.Errorf("reader was called %d times across two calls in one process, want 1", reader.calls)
	}
}

// TestApplyVaultAuthSource_MemoizedErrorIsAlsoCached confirms a failed first
// call doesn't retry the Vault read on a second call within the same
// process either — the inputs (mount/path/prefix) can't have changed, so a
// retry would just fail the same way while doing another network round trip.
func TestApplyVaultAuthSource_MemoizedErrorIsAlsoCached(t *testing.T) {
	resetVaultTestState(t)
	viper.Set("vault-kv-mount", "osd-sre")
	viper.Set("vault-kv-path", "rosa-agent")

	reader := &fakeVaultReader{err: &vaultconfig.ErrForbidden{Mount: "osd-sre", Path: "rosa-agent"}}
	ctx := withFakeVaultReader(reader)

	err1 := applyVaultAuthSource(ctx)
	err2 := applyVaultAuthSource(ctx)
	var forbidden *vaultconfig.ErrForbidden
	if !errors.As(err1, &forbidden) || !errors.As(err2, &forbidden) {
		t.Fatalf("expected both calls to return *vaultconfig.ErrForbidden, got %v and %v", err1, err2)
	}
	if reader.calls != 1 {
		t.Errorf("reader was called %d times across two calls in one process, want 1", reader.calls)
	}
}

// TestVersionCommand_NeverTouchesVaultEvenWhenConfigured pins the acceptance
// criterion that only commands resolving auth touch Vault: `version` reads
// none of the affected viper keys, so it must succeed without ever invoking
// a reader, even when --vault-kv-mount/--vault-kv-path are set (e.g. from a
// user's shell profile) and would otherwise fail fast on a missing Vault
// session. If this regresses (e.g. someone moves applyVaultAuthSource into
// root.PersistentPreRunE), this test starts failing because the injected
// fake reader records a call that must never happen.
func TestVersionCommand_NeverTouchesVaultEvenWhenConfigured(t *testing.T) {
	reader := &fakeVaultReader{err: errors.New("must not be called")}
	ctx := withVaultDeps(context.Background(), vaultDeps{Reader: reader})

	out, err := runCmdCtx(t, ctx, "--vault-kv-mount", "osd-sre", "--vault-kv-path", "rosa-agent", "version")
	if err != nil {
		t.Fatalf("version: %v\noutput:\n%s", err, out)
	}
	if reader.calls != 0 {
		t.Errorf("reader was called %d times, want 0 — version has no auth dependency", reader.calls)
	}
}

// TestGatewayLogout_PicksUpVaultSuppliedGatewayName is the one true end-to-end
// smoke test for the real wiring: it runs an actual command (gateway logout)
// through root.Execute() with no -g/--gateway flag and no OPENSHELL_GATEWAY
// env var, relying entirely on a Vault-injected default for the gateway name.
// A negative-control run without the Vault deps injected confirms the
// positive case genuinely depends on Vault, not on some other fallback
// (e.g. an active_gateway file, which this setup deliberately omits).
func TestGatewayLogout_PicksUpVaultSuppliedGatewayName(t *testing.T) {
	setupLogoutGatewayTree(t)

	reader := &fakeVaultReader{data: map[string]any{"gateway": "rosa"}}
	ctx := withVaultDeps(context.Background(), vaultDeps{Reader: reader})
	out, err := runCmdCtx(t, ctx, "--vault-kv-mount", "osd-sre", "--vault-kv-path", "rosa-agent", "gateway", "logout")
	if err != nil {
		t.Fatalf("gateway logout: %v\noutput:\n%s", err, out)
	}
	if reader.calls != 1 {
		t.Errorf("reader was called %d times, want 1", reader.calls)
	}
}

func TestGatewayLogout_WithoutVaultFailsWithNoActiveGateway(t *testing.T) {
	setupLogoutGatewayTree(t)

	_, err := runCmdCtx(t, context.Background(), "gateway", "logout")
	if err == nil {
		t.Fatal("expected an error without a Vault-supplied or active gateway name")
	}
}

// setupLogoutGatewayTree writes a registered gateway named "rosa" but
// deliberately does NOT write an active_gateway file, so resolving the
// gateway name has no fallback other than an explicit flag/env/Vault value.
func setupLogoutGatewayTree(t *testing.T) {
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
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", xdg)
}
