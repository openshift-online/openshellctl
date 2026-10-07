package cli

import (
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/gateway/mock"
)

// TestSandboxList_PermissionDenied_ExitForbidden runs `sandbox list` end to
// end through the real command tree (via the cliDeps mock-gateway seam, same
// pattern as TestSandboxList_WithMockGateway) against a gateway that returns
// PermissionDeniedError, and confirms the resulting error classifies to the
// new ExitForbidden (7) and gets the distinct, accurate hint — not the
// generic "try token refresh" hint a plain auth failure would get. This is
// the third onboarding-thread symptom from ROSAENG-68828, reproduced as a
// genuine CLI-level test rather than only a unit test of exitCodeFor/hintFor
// in isolation.
func TestSandboxList_PermissionDenied_ExitForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().
		ListSandboxes(gomock.Any(), "default", gomock.Any()).
		Return(nil, &gateway.PermissionDeniedError{Message: "role \"openshell-admin\" required"})

	_, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "sandbox", "list")
	if err == nil {
		t.Fatal("expected an error")
	}
	var denied *gateway.PermissionDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("err = %v, want *gateway.PermissionDeniedError", err)
	}
	if code := exitCodeFor(err); code != ExitForbidden {
		t.Errorf("exitCodeFor(err) = %d, want ExitForbidden", code)
	}
	hint := hintFor(err)
	if hint == "" {
		t.Fatal("expected a non-empty hint for permission-denied")
	}
	if contains(hint, "token refresh") {
		t.Errorf("permission-denied hint must not suggest token refresh, got: %q", hint)
	}
}
