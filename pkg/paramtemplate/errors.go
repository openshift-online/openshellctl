package paramtemplate

import (
	"fmt"
	"strings"
)

// MalformedParamError reports a --param or --param-file entry that is not
// valid KEY=VALUE: either there is no '=', or KEY does not match
// [A-Za-z_][A-Za-z0-9_]*. Source is "--param" for a flag entry, or the
// --param-file path for a file entry (with Line set to its 1-based line
// number).
type MalformedParamError struct {
	Raw    string
	Source string
	Line   int
}

func (e *MalformedParamError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d: invalid parameter %q, expected KEY=VALUE with KEY matching [A-Za-z_][A-Za-z0-9_]*",
			e.Source, e.Line, e.Raw)
	}
	return fmt.Sprintf("%s: invalid parameter %q, expected KEY=VALUE with KEY matching [A-Za-z_][A-Za-z0-9_]*",
		e.Source, e.Raw)
}

// ParamFileUnreadableError wraps a failure to open or read a --param-file.
type ParamFileUnreadableError struct {
	Path string
	Err  error
}

func (e *ParamFileUnreadableError) Error() string {
	return fmt.Sprintf("reading --param-file %q: %v", e.Path, e.Err)
}

func (e *ParamFileUnreadableError) Unwrap() error { return e.Err }

// UnresolvedParamsError lists every ${KEY} reference left unresolved after
// substitution (sorted, deduplicated) — a manifest must never be sent to the
// gateway with a literal ${KEY} in it.
type UnresolvedParamsError struct{ Keys []string }

func (e *UnresolvedParamsError) Error() string {
	return fmt.Sprintf("unresolved parameter reference(s): %s", strings.Join(e.Keys, ", "))
}

// UnknownParamsError lists --param/--param-file keys the manifest does not
// reference (sorted) — triggered unless --ignore-unknown-parameters is set.
type UnknownParamsError struct{ Keys []string }

func (e *UnknownParamsError) Error() string {
	return fmt.Sprintf("parameter(s) not referenced by manifest: %s (use --ignore-unknown-parameters to allow)",
		strings.Join(e.Keys, ", "))
}
