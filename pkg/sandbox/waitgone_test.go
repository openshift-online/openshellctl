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

// stillThere is returned by a fake GetSandbox call to mean "exists, not yet
// deleted" — a successful Get (sandbox, nil error), not an error. A non-nil,
// non-NotFound error is a different, real failure (see
// TestWaitGone_RPCErrorPropagatesImmediately) and must never be confused
// with "still there" — that was exactly the bug a review caught here.
func stillThere() (*types.Sandbox, error) { return &types.Sandbox{Name: "sb"}, nil }

// TestWaitGone_GoneOnThirdPoll drives the poll loop with a manually-filled
// Tick channel — no real sleep — to deterministically exercise "still there,
// still there, gone" without any wall-clock delay.
func TestWaitGone_GoneOnThirdPoll(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gomock.InOrder(
		gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").DoAndReturn(func(context.Context, string, string) (*types.Sandbox, error) { return stillThere() }),
		gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").DoAndReturn(func(context.Context, string, string) (*types.Sandbox, error) { return stillThere() }),
		gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").
			Return((*types.Sandbox)(nil), &gateway.NotFoundError{Resource: "sandbox", Name: "sb"}),
	)

	tick := make(chan time.Time, 2)
	tick <- time.Now()
	tick <- time.Now()

	err := waitGone(context.Background(), gw, "default", "sb", time.Minute, waitGoneDeps{Tick: tick})
	if err != nil {
		t.Fatalf("waitGone: %v", err)
	}
}

// TestWaitGone_Timeout drives a Clock that's already past the deadline by
// the second call — no real sleep, no real ticks needed (Tick is non-nil but
// never has to fire since the timeout is caught before the select).
func TestWaitGone_Timeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").
		DoAndReturn(func(context.Context, string, string) (*types.Sandbox, error) { return stillThere() }).AnyTimes()

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	clock := func() time.Time {
		calls++
		if calls == 1 {
			return start // establishes the deadline
		}
		return start.Add(time.Hour) // every subsequent call is already past it
	}

	err := waitGone(context.Background(), gw, "default", "sb", time.Minute, waitGoneDeps{Clock: clock, Tick: make(chan time.Time)})
	var timeoutErr *ErrDeleteTimeout
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("err = %v, want *ErrDeleteTimeout", err)
	}
}

// TestWaitGone_ContextCancel confirms a cancelled context is returned
// promptly rather than waiting for a tick that will never come.
func TestWaitGone_ContextCancel(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").
		DoAndReturn(func(context.Context, string, string) (*types.Sandbox, error) { return stillThere() }).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitGone(ctx, gw, "default", "sb", time.Minute, waitGoneDeps{Tick: make(chan time.Time)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// TestWaitGone_RPCErrorPropagatesImmediately is the review-caught
// regression: a real RPC error from GetSandbox (Unauthenticated,
// Unavailable, ...) is not "still there" and must not be silently retried
// for the whole timeout window — it must propagate immediately, with no
// further Get attempted and no *ErrDeleteTimeout masking it. Reproduces the
// exact scenario the review described: a token that expires mid-wait.
func TestWaitGone_RPCErrorPropagatesImmediately(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	wantErr := &gateway.UnauthenticatedError{Message: "token expired"}
	gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").Return((*types.Sandbox)(nil), wantErr)
	// No further GetSandbox expectation — gomock fails the test if waitGone
	// keeps polling instead of propagating immediately.

	err := waitGone(context.Background(), gw, "default", "sb", time.Minute, waitGoneDeps{Tick: make(chan time.Time)})
	if err != wantErr { //nolint:errorlint // direct identity: waitGone must return the GetSandbox error unwrapped
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	var timeoutErr *ErrDeleteTimeout
	if errors.As(err, &timeoutErr) {
		t.Fatal("waitGone returned *ErrDeleteTimeout, want the real RPC error — the timeout must never mask it")
	}
}

// TestWaitGone_DefaultsAreSafe confirms a zero-value waitGoneDeps (the
// production shape for the existing Delete --wait path) still works — a
// real ticker, real clock — by using a target that's NotFound on the very
// first check so the test completes instantly regardless.
func TestWaitGone_DefaultsAreSafe(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().GetSandbox(gomock.Any(), "default", "sb").Return((*types.Sandbox)(nil), &gateway.NotFoundError{Resource: "sandbox", Name: "sb"})

	if err := waitGone(context.Background(), gw, "default", "sb", time.Minute, waitGoneDeps{}); err != nil {
		t.Fatalf("waitGone with zero-value deps: %v", err)
	}
}
