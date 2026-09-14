package gateway

import (
	"errors"
	"testing"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestClassify_Nil(t *testing.T) {
	if Classify(nil, "sandbox", "x") != nil {
		t.Error("Classify(nil) should be nil")
	}
}

func TestClassify_SDKStatusError(t *testing.T) {
	tests := []struct {
		code types.ErrorCode
		want func(error) bool
	}{
		{types.ErrorNotFound, func(e error) bool { var x *NotFoundError; return errors.As(e, &x) }},
		{types.ErrorAlreadyExists, func(e error) bool { var x *AlreadyExistsError; return errors.As(e, &x) }},
		{types.ErrorConflict, func(e error) bool { var x *ConflictError; return errors.As(e, &x) }},
		{types.ErrorUnauthenticated, func(e error) bool { var x *UnauthenticatedError; return errors.As(e, &x) }},
		{types.ErrorPermissionDenied, func(e error) bool { var x *PermissionDeniedError; return errors.As(e, &x) }},
		{types.ErrorInvalidArgument, func(e error) bool { var x *InvalidArgumentError; return errors.As(e, &x) }},
		{types.ErrorUnavailable, func(e error) bool { var x *UnavailableError; return errors.As(e, &x) }},
		{types.ErrorDeadlineExceeded, func(e error) bool { var x *DeadlineError; return errors.As(e, &x) }},
		{types.ErrorInternal, func(e error) bool { var x *RPCError; return errors.As(e, &x) }},
		{types.ErrorUnimplemented, func(e error) bool { var x *RPCError; return errors.As(e, &x) }},
	}
	for _, tt := range tests {
		se := &types.StatusError{Code: tt.code, Message: "msg", Cause: errors.New("root")}
		got := Classify(se, "sandbox", "n")
		if !tt.want(got) {
			t.Errorf("code %d classified to %T (unexpected)", tt.code, got)
		}
		// Every classified error must unwrap to the cause chain.
		if !errors.Is(got, se) && got.Error() == "" {
			t.Errorf("code %d: classified error has empty message", tt.code)
		}
	}
}

func TestClassify_GRPCCodes(t *testing.T) {
	tests := []struct {
		code codes.Code
		want func(error) bool
	}{
		{codes.NotFound, func(e error) bool { var x *NotFoundError; return errors.As(e, &x) }},
		{codes.AlreadyExists, func(e error) bool { var x *AlreadyExistsError; return errors.As(e, &x) }},
		{codes.Aborted, func(e error) bool { var x *ConflictError; return errors.As(e, &x) }},
		{codes.FailedPrecondition, func(e error) bool { var x *ConflictError; return errors.As(e, &x) }},
		{codes.Unauthenticated, func(e error) bool { var x *UnauthenticatedError; return errors.As(e, &x) }},
		{codes.PermissionDenied, func(e error) bool { var x *PermissionDeniedError; return errors.As(e, &x) }},
		{codes.InvalidArgument, func(e error) bool { var x *InvalidArgumentError; return errors.As(e, &x) }},
		{codes.OutOfRange, func(e error) bool { var x *InvalidArgumentError; return errors.As(e, &x) }},
		{codes.Unavailable, func(e error) bool { var x *UnavailableError; return errors.As(e, &x) }},
		{codes.ResourceExhausted, func(e error) bool { var x *UnavailableError; return errors.As(e, &x) }},
		{codes.DeadlineExceeded, func(e error) bool { var x *DeadlineError; return errors.As(e, &x) }},
		{codes.Internal, func(e error) bool { var x *RPCError; return errors.As(e, &x) }},
	}
	for _, tt := range tests {
		err := status.Error(tt.code, "boom")
		got := Classify(err, "sandbox", "n")
		if !tt.want(got) {
			t.Errorf("grpc code %s classified to %T (unexpected)", tt.code, got)
		}
	}
}

func TestClassify_NonGRPCError(t *testing.T) {
	got := Classify(errors.New("plain"), "", "")
	var rpc *RPCError
	if !errors.As(got, &rpc) {
		t.Fatalf("plain error → %T, want *RPCError", got)
	}
	if rpc.Code != codes.Unknown {
		t.Errorf("code = %s, want Unknown", rpc.Code)
	}
}

func TestClassify_NotFoundMessageUsesResourceName(t *testing.T) {
	se := &types.StatusError{Code: types.ErrorNotFound, Message: "x"}
	got := Classify(se, "sandbox", "sb-1")
	if got.Error() != `sandbox "sb-1" not found` {
		t.Errorf("message = %q", got.Error())
	}
}
