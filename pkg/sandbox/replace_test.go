package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"go.uber.org/mock/gomock"

	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/gateway/mock"
)

// TestReplaceExisting_NotFound confirms nothing happens — no Delete call at
// all — when there's nothing to replace.
func TestReplaceExisting_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").
		Return((*types.Sandbox)(nil), &gateway.NotFoundError{Resource: "sandbox", Name: "sb"})
	// No DeleteSandbox expectation at all — gomock fails the test if it's called.

	if err := replaceExisting(context.Background(), gw, "default", "sb", time.Minute, nil); err != nil {
		t.Fatalf("replaceExisting: %v", err)
	}
}

// TestReplaceExisting_FoundDeletedGone confirms the full happy path in
// order: Get (found) -> Delete -> Get (NotFound, confirming gone).
func TestReplaceExisting_FoundDeletedGone(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gomock.InOrder(
		gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").Return(&types.Sandbox{Name: "sb"}, nil),
		gw.EXPECT().DeleteSandbox(gomock.Any(), "default", "sb").Return(true, nil),
		gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").
			Return((*types.Sandbox)(nil), &gateway.NotFoundError{Resource: "sandbox", Name: "sb"}),
	)

	if err := replaceExisting(context.Background(), gw, "default", "sb", time.Minute, nil); err != nil {
		t.Fatalf("replaceExisting: %v", err)
	}
}

// TestReplaceExisting_GetRPCError confirms a Get RPC error (distinct from
// NotFound) propagates without attempting a Delete.
func TestReplaceExisting_GetRPCError(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	wantErr := &gateway.UnavailableError{Message: "gateway down"}
	gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").Return((*types.Sandbox)(nil), wantErr)

	err := replaceExisting(context.Background(), gw, "default", "sb", time.Minute, nil)
	if !errors.Is(err, wantErr) && err != wantErr { //nolint:errorlint // UnavailableError has no Is/Unwrap; direct identity is correct here
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

// TestReplaceExisting_DeleteRPCErrorPropagates is the ticket's second
// acceptance criterion at the pkg/sandbox layer: a delete RPC failure during
// replace propagates as that error, with no further Get attempted.
func TestReplaceExisting_DeleteRPCErrorPropagates(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	wantErr := &gateway.UnauthenticatedError{Message: "token expired"}
	gomock.InOrder(
		gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").Return(&types.Sandbox{Name: "sb"}, nil),
		gw.EXPECT().DeleteSandbox(gomock.Any(), "default", "sb").Return(false, wantErr),
	)

	err := replaceExisting(context.Background(), gw, "default", "sb", time.Minute, nil)
	if err != wantErr { //nolint:errorlint // direct identity: replaceExisting must return the DeleteSandbox error unwrapped
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

// TestReplaceExisting_NeverGoneTimeout confirms a delete that never actually
// completes (GetSandbox keeps finding it) surfaces waitGone's
// *ErrDeleteTimeout, not a fake success.
func TestReplaceExisting_NeverGoneTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").Return(&types.Sandbox{Name: "sb"}, nil)
	gw.EXPECT().DeleteSandbox(gomock.Any(), "default", "sb").Return(true, nil)
	gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").
		Return(&types.Sandbox{Name: "sb"}, nil).AnyTimes() // still there, forever

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	clock := func() time.Time {
		calls++
		if calls == 1 {
			return start
		}
		return start.Add(time.Hour)
	}

	err := replaceExistingWithDeps(context.Background(), gw, "default", "sb", time.Minute, waitGoneDeps{Clock: clock, Tick: make(chan time.Time)})
	var timeoutErr *ErrDeleteTimeout
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("err = %v, want *ErrDeleteTimeout", err)
	}
}

// TestReplaceExisting_ContextCancel confirms a cancelled context aborts the
// wait-for-gone step promptly.
func TestReplaceExisting_ContextCancel(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").Return(&types.Sandbox{Name: "sb"}, nil)
	gw.EXPECT().DeleteSandbox(gomock.Any(), "default", "sb").Return(true, nil)
	gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").Return(&types.Sandbox{Name: "sb"}, nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := replaceExistingWithDeps(ctx, gw, "default", "sb", time.Minute, waitGoneDeps{Tick: make(chan time.Time)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
