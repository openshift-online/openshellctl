package cli

import (
	"errors"
	"fmt"

	"github.com/openshift-online/openshellctl/pkg/auth"
	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
	"github.com/openshift-online/openshellctl/pkg/sandbox"
)

// Exit codes, per the implementation spec §5.9.
const (
	ExitOK        = 0 // success
	ExitError     = 1 // generic / RPC error
	ExitUsage     = 2 // usage: flag parse, validation, unsupported flag combos
	ExitAuth      = 3 // authentication failures
	ExitNotFound  = 4 // sandbox/gateway not found
	ExitConflict  = 5 // conflict / already-exists
	ExitProvision = 6 // provisioning timeout/failure, lifecycle
)

// UsageError marks an error as a usage error (exit code 2). Command layers wrap
// flag/validation problems in this so Execute maps them to ExitUsage.
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// NotImplementedError is returned by command stubs that are not yet wired up.
// It maps to ExitError. It exists so the full command tree can be built (and
// enumerated by the parity test) before every command is implemented.
type NotImplementedError struct{ Command string }

func (e *NotImplementedError) Error() string {
	return e.Command + ": not implemented yet"
}

// RemoteExitError carries the exit code of a remote process (exec/connect/create
// with an attached command). It maps directly to that code so the local process
// exits with the remote's status. It is only constructed for non-zero codes.
type RemoteExitError struct{ Code int }

func (e *RemoteExitError) Error() string {
	return fmt.Sprintf("remote process exited with status %d", e.Code)
}

// exitCodeError returns a *RemoteExitError for a non-zero remote code, or nil for
// zero (so the command succeeds silently on exit 0).
func exitCodeError(code int) error {
	if code == 0 {
		return nil
	}
	return &RemoteExitError{Code: code}
}

// exitCodeFor maps an error returned by a command to a process exit code.
// As typed errors from pkg/* land in later commits, they are classified here.
func exitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	var remote *RemoteExitError
	if errors.As(err, &remote) {
		return remote.Code
	}
	var usage *UsageError
	if errors.As(err, &usage) {
		return ExitUsage
	}

	// Authentication failures → 3.
	var expired *auth.ErrTokenExpired
	var exchange *auth.ExchangeError
	var oidcMissing *auth.ErrOIDCConfigMissing
	var gwUnauth *gateway.UnauthenticatedError
	var gwPerm *gateway.PermissionDeniedError
	if errors.As(err, &expired) || errors.As(err, &exchange) || errors.As(err, &oidcMissing) ||
		errors.As(err, &gwUnauth) || errors.As(err, &gwPerm) {
		return ExitAuth
	}

	// Not found → 4 (gateway-config resolution and gateway RPC NotFound).
	var gwNotFound *gateway.NotFoundError
	if errors.Is(err, gatewayconfig.ErrGatewayNotFound) ||
		errors.Is(err, gatewayconfig.ErrUnknownGateway) ||
		errors.Is(err, gatewayconfig.ErrNoActiveGateway) ||
		errors.As(err, &gwNotFound) {
		return ExitNotFound
	}

	// Conflict / already-exists → 5.
	var gwExists *gateway.AlreadyExistsError
	var gwConflict *gateway.ConflictError
	if errors.As(err, &gwExists) || errors.As(err, &gwConflict) {
		return ExitConflict
	}

	// Provisioning / lifecycle → 6.
	var provFailed *sandbox.ErrProvisionFailed
	var provTimeout *sandbox.ErrProvisionTimeout
	var lifecycle *sandbox.ErrLifecycle
	var lifecycleTimeout *sandbox.ErrLifecycleTimeout
	var lifecycleEnded *sandbox.ErrLifecycleStreamEnded
	if errors.As(err, &provFailed) || errors.As(err, &provTimeout) ||
		errors.As(err, &lifecycle) || errors.As(err, &lifecycleTimeout) ||
		errors.As(err, &lifecycleEnded) {
		return ExitProvision
	}

	return ExitError
}
