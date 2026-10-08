package doctor

import "fmt"

// ErrChecksFailed is returned by internal/cli/doctor.go's RunE when Run
// reports at least one failing check. It carries every result (not just
// failures) so a renderer prints the full table from the error alone, and
// Results lets ExitCode() pick the exit code implied by the specific checks
// that failed.
type ErrChecksFailed struct {
	Results []CheckResult
}

func (e *ErrChecksFailed) Error() string {
	n := 0
	for _, r := range e.Results {
		if r.Status == StatusFail {
			n++
		}
	}
	return fmt.Sprintf("%d doctor check(s) failed", n)
}

// Exit code values, mirroring internal/cli's ExitError/ExitUsage/ExitAuth/
// ExitForbidden constants (internal/cli/exitcode.go) by value, not by
// import: internal/cli imports this package to wire the `doctor` command,
// so the reverse import would cycle. internal/cli/exitcode_test.go pins
// these against the real constants so the two can't silently drift apart.
const (
	exitError     = 1
	exitUsage     = 2
	exitAuth      = 3
	exitForbidden = 7
)

// ExitCode returns the process exit code for this failure, picked by
// priority among the failing checks — the same semantic
// internal/cli/exitcode.go already assigns the underlying problems each
// check predicts:
//   - Roles failing predicts a real gateway PermissionDeniedError →
//     exitForbidden (checked first: authorization, not expiry — a token
//     refresh can't fix it, same reasoning as hintFor's PermissionDeniedError
//     case).
//   - Credentials, Audience, or Expiry failing is an authentication problem
//     → exitAuth.
//   - Endpoint URL (malformed input) or Providers (an unrecognized/
//     not-yet-created name) failing is a usage/configuration problem →
//     exitUsage.
//   - Anything else failing (DNS, HTTP reachability, OIDC config match) is
//     a generic environment problem, not something a flag or re-auth fixes
//     → exitError.
//
// Returns 0 when nothing failed (ErrChecksFailed should not have been
// constructed in that case, but ExitCode is still well-defined).
func (e *ErrChecksFailed) ExitCode() int {
	status := make(map[string]Status, len(e.Results))
	anyFail := false
	for _, r := range e.Results {
		status[r.Name] = r.Status
		if r.Status == StatusFail {
			anyFail = true
		}
	}
	if !anyFail {
		return 0
	}
	switch {
	case status["Roles"] == StatusFail:
		return exitForbidden
	case status["Credentials"] == StatusFail, status["Audience"] == StatusFail, status["Expiry"] == StatusFail:
		return exitAuth
	case status["Endpoint URL"] == StatusFail, status["Providers"] == StatusFail:
		return exitUsage
	default:
		return exitError
	}
}
