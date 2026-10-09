// Package doctor implements openshellctl's connectivity/auth preflight
// checks (ROSAENG-68830): a fixed sequence of named checks, each producing a
// pass/fail/skip result with a human detail line and, for a failure, the
// exact next command to run. It turns a multi-person Slack debugging thread
// into a single command's output.
//
// The package is split by I/O shape: types.go (this file) defines the
// shared result/status vocabulary and the Deps injection seam; checks_pure.go
// holds checks that are plain functions over already-fetched data (no I/O,
// exhaustively table-tested); checks_io.go holds the two checks that need a
// real network call, each taking an injected function so tests never touch
// the network; run.go orchestrates the fixed sequence and its skip
// semantics; errors.go defines the error Run's caller returns and the
// exit-code priority among failing checks.
package doctor

// Status is a single check's outcome.
type Status string

// The three possible outcomes for a check. Skip is a first-class status, not
// an early-exit: a skipped check still produces a CheckResult and a line in
// the rendered output, with Detail explaining why it didn't run (an
// upstream check failed, or it wasn't applicable/requested).
const (
	StatusPass Status = "pass"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// CheckResult is one check's outcome.
type CheckResult struct {
	// Name is the check's human label, e.g. "Credentials" — stable across
	// releases since scripts may key off it in JSON/YAML output.
	Name string `json:"name"`
	// Status is one of StatusPass/StatusFail/StatusSkip.
	Status Status `json:"status"`
	// Detail is a one-line human description of what was checked and what
	// was found — populated for every status (including skip, which
	// explains why).
	Detail string `json:"detail"`
	// NextStep is the exact command (or short instruction) to run to fix a
	// failure. Empty for pass/skip.
	NextStep string `json:"nextStep,omitempty"`
}
