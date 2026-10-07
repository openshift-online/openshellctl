package cli

import (
	"strings"
	"testing"
	"time"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"go.uber.org/mock/gomock"

	"github.com/openshift-online/openshellctl/pkg/gateway/mock"
)

// TestSandboxList_WithMockGateway runs `sandbox list` end to end through
// root.Execute() against a mock.MockGateway injected via the cliDeps seam
// (deps.go) — no real gateway resolution or dial happens. This is the
// story's "at least one command tested end-to-end against the mock gateway"
// acceptance criterion.
func TestSandboxList_WithMockGateway(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)

	sb := &types.Sandbox{ID: "id-demo", Name: "demo", Workspace: "default"}
	sb.CreatedAt = time.UnixMilli(1700000000000)
	sb.Status.Phase = types.SandboxReady

	gw.EXPECT().
		ListSandboxes(gomock.Any(), "default", gomock.Any()).
		Return([]*types.Sandbox{sb}, nil)

	out, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "sandbox", "list")
	if err != nil {
		t.Fatalf("sandbox list: %v", err)
	}
	if !strings.Contains(out, "demo") {
		t.Errorf("sandbox list output missing sandbox name; got:\n%s", out)
	}
}
