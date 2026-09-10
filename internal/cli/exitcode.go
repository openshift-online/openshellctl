package cli

import "errors"

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

// exitCodeFor maps an error returned by a command to a process exit code.
// As typed errors from pkg/* land in later commits, they are classified here.
func exitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	var usage *UsageError
	if errors.As(err, &usage) {
		return ExitUsage
	}
	return ExitError
}
