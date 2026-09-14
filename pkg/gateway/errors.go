package gateway

import (
	"errors"
	"fmt"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Typed error taxonomy. Every SDK or raw error is wrapped once through Classify
// into one of these; each carries the original as Cause and implements Unwrap.

// NotFoundError reports a missing resource (sandbox/provider/gateway).
type NotFoundError struct {
	Resource string
	Name     string
	Cause    error
}

func (e *NotFoundError) Error() string {
	if e.Resource != "" || e.Name != "" {
		return fmt.Sprintf("%s %q not found", e.Resource, e.Name)
	}
	return "not found"
}
func (e *NotFoundError) Unwrap() error { return e.Cause }

// AlreadyExistsError reports a create conflict on an existing name.
type AlreadyExistsError struct {
	Name    string
	Message string
	Cause   error
}

func (e *AlreadyExistsError) Error() string { return e.Message }
func (e *AlreadyExistsError) Unwrap() error { return e.Cause }

// ConflictError reports Aborted/FailedPrecondition/Conflict (e.g. resource
// version mismatch on attach/detach).
type ConflictError struct {
	Message string
	Cause   error
}

func (e *ConflictError) Error() string { return e.Message }
func (e *ConflictError) Unwrap() error { return e.Cause }

// UnauthenticatedError reports a missing/invalid/expired token.
type UnauthenticatedError struct {
	Message string
	Cause   error
}

func (e *UnauthenticatedError) Error() string { return e.Message }
func (e *UnauthenticatedError) Unwrap() error { return e.Cause }

// PermissionDeniedError reports an authenticated caller lacking a required role.
type PermissionDeniedError struct {
	Message string
	Cause   error
}

func (e *PermissionDeniedError) Error() string { return e.Message }
func (e *PermissionDeniedError) Unwrap() error { return e.Cause }

// InvalidArgumentError reports a client-side/validation error rejected by the gateway.
type InvalidArgumentError struct {
	Message string
	Cause   error
}

func (e *InvalidArgumentError) Error() string { return e.Message }
func (e *InvalidArgumentError) Unwrap() error { return e.Cause }

// UnavailableError reports the gateway being unreachable.
type UnavailableError struct {
	Message string
	Cause   error
}

func (e *UnavailableError) Error() string { return e.Message }
func (e *UnavailableError) Unwrap() error { return e.Cause }

// DeadlineError reports a deadline exceeded.
type DeadlineError struct{ Cause error }

func (e *DeadlineError) Error() string { return "deadline exceeded" }
func (e *DeadlineError) Unwrap() error { return e.Cause }

// RPCError is the catch-all for codes without a dedicated type.
type RPCError struct {
	Code    codes.Code
	Message string
	Cause   error
}

func (e *RPCError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
func (e *RPCError) Unwrap() error { return e.Cause }

// Classify wraps an SDK or raw gRPC error into the typed taxonomy. SDK errors
// carry a *types.StatusError (its Code drives classification); raw errors are
// classified via status.FromError. A nil error returns nil.
//
// resource/name annotate a resulting NotFoundError for a nicer message; pass ""
// when unknown.
func Classify(err error, resource, name string) error {
	if err == nil {
		return nil
	}

	var se *types.StatusError
	if errors.As(err, &se) {
		return classifySDKCode(se.Code, se.Message, resource, name, err)
	}

	if st, ok := status.FromError(err); ok {
		return classifyGRPCCode(st.Code(), st.Message(), resource, name, err)
	}

	return &RPCError{Code: codes.Unknown, Message: err.Error(), Cause: err}
}

func classifySDKCode(code types.ErrorCode, msg, resource, name string, cause error) error {
	switch code {
	case types.ErrorNotFound:
		return &NotFoundError{Resource: resource, Name: name, Cause: cause}
	case types.ErrorAlreadyExists:
		return &AlreadyExistsError{Name: name, Message: msg, Cause: cause}
	case types.ErrorConflict:
		return &ConflictError{Message: msg, Cause: cause}
	case types.ErrorUnauthenticated:
		return &UnauthenticatedError{Message: msg, Cause: cause}
	case types.ErrorPermissionDenied:
		return &PermissionDeniedError{Message: msg, Cause: cause}
	case types.ErrorInvalidArgument:
		return &InvalidArgumentError{Message: msg, Cause: cause}
	case types.ErrorUnavailable:
		return &UnavailableError{Message: msg, Cause: cause}
	case types.ErrorDeadlineExceeded:
		return &DeadlineError{Cause: cause}
	default:
		return &RPCError{Code: codes.Unknown, Message: msg, Cause: cause}
	}
}

func classifyGRPCCode(code codes.Code, msg, resource, name string, cause error) error {
	switch code {
	case codes.NotFound:
		return &NotFoundError{Resource: resource, Name: name, Cause: cause}
	case codes.AlreadyExists:
		return &AlreadyExistsError{Name: name, Message: msg, Cause: cause}
	case codes.Aborted, codes.FailedPrecondition:
		return &ConflictError{Message: msg, Cause: cause}
	case codes.Unauthenticated:
		return &UnauthenticatedError{Message: msg, Cause: cause}
	case codes.PermissionDenied:
		return &PermissionDeniedError{Message: msg, Cause: cause}
	case codes.InvalidArgument, codes.OutOfRange:
		return &InvalidArgumentError{Message: msg, Cause: cause}
	case codes.Unavailable, codes.ResourceExhausted:
		return &UnavailableError{Message: msg, Cause: cause}
	case codes.DeadlineExceeded:
		return &DeadlineError{Cause: cause}
	case codes.OK:
		return nil
	default:
		return &RPCError{Code: code, Message: msg, Cause: cause}
	}
}
