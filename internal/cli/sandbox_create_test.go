package cli

import (
	"strings"
	"testing"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"go.uber.org/mock/gomock"

	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/gateway/mock"
)

// TestSandboxCreate_Replace runs `sandbox create --replace` end to end
// through the real command tree via the Feature 0 cliDeps seam
// (mock.MockGateway) — an ordered Get→Delete→Get-NotFound→Create sequence,
// matching pkg/sandbox's own TestCreate_Replace but exercising the full CLI
// flag/validation path on top.
func TestSandboxCreate_Replace(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gomock.InOrder(
		gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").Return(&types.Sandbox{Name: "sb"}, nil),
		gw.EXPECT().DeleteSandbox(gomock.Any(), "default", "sb").Return(true, nil),
		gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").
			Return((*types.Sandbox)(nil), &gateway.NotFoundError{Resource: "sandbox", Name: "sb"}),
		gw.EXPECT().CreateSandbox(gomock.Any(), "default", "sb", gomock.Any(), gomock.Any()).
			Return(&types.Sandbox{ID: "id-2", Name: "sb"}, nil),
	)

	out, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "sandbox", "create",
		"--name", "sb", "--from", "python", "--replace", "--detach", "-o", "json")
	if err != nil {
		t.Fatalf("sandbox create --replace: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "id-2") {
		t.Errorf("expected the freshly-created sandbox in output, got:\n%s", out)
	}
}

// TestSandboxCreate_Replace_DeleteRPCError_NonZeroExit is the ticket's
// second acceptance criterion end to end through the CLI: a delete RPC
// failure during --replace must exit with that error's real code, not 0,
// and Create must never be called.
func TestSandboxCreate_Replace_DeleteRPCError_NonZeroExit(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gomock.InOrder(
		gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").Return(&types.Sandbox{Name: "sb"}, nil),
		gw.EXPECT().DeleteSandbox(gomock.Any(), "default", "sb").Return(false, &gateway.UnauthenticatedError{Message: "token expired"}),
	)
	// No CreateSandbox expectation — gomock fails the test if it's called.

	_, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "sandbox", "create",
		"--name", "sb", "--from", "python", "--replace", "--detach")
	if err == nil {
		t.Fatal("expected an error")
	}
	if exitCodeFor(err) == ExitOK {
		t.Errorf("exit = %d, want non-zero (the delete RPC error's real code)", exitCodeFor(err))
	}
	if exitCodeFor(err) != ExitAuth {
		t.Errorf("exit = %d, want ExitAuth (3, UnauthenticatedError's mapping)", exitCodeFor(err))
	}
}

// TestSandboxCreate_ReplaceWithoutName_UsageError confirms --replace with no
// resolvable name (no --name, no manifest, server-generated name) fails
// fast with a usage error — mirroring the manifest-side
// "replace requires metadata.name" rule for the flag-only path.
func TestSandboxCreate_ReplaceWithoutName_UsageError(t *testing.T) {
	_, err := runCmd(t, "sandbox", "create", "--from", "python", "--replace")
	if err == nil {
		t.Fatal("expected a usage error")
	}
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want ExitUsage (2)", exitCodeFor(err))
	}
	if !strings.Contains(err.Error(), "--replace") {
		t.Errorf("error should mention --replace, got: %v", err)
	}
}

// TestSandboxCreate_ReplaceTimeoutFlag_Registered confirms --replace-timeout
// parses as a duration flag (a bare registration/parsing smoke test — the
// actual default-applies-when-Replace-is-set behavior is covered by
// pkg/sandbox/mergespec_test.go).
func TestSandboxCreate_ReplaceTimeoutFlag_Registered(t *testing.T) {
	_, err := runCmd(t, "sandbox", "create", "--replace-timeout", "notaduration")
	if err == nil {
		t.Fatal("expected a flag-parse error for an invalid duration")
	}
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want ExitUsage (2, cobra flag parse errors map to usage)", exitCodeFor(err))
	}
}
