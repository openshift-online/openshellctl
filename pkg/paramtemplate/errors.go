package paramtemplate

import "fmt"

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
