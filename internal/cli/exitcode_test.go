package cli

import (
	"errors"
	"testing"
)

func TestExitCodeFor(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil is ok", nil, ExitOK},
		{"generic error", errors.New("boom"), ExitError},
		{"usage error", &UsageError{Err: errors.New("bad flag")}, ExitUsage},
		{"wrapped usage error", errWrap(&UsageError{Err: errors.New("bad flag")}), ExitUsage},
		{"not implemented is generic", &NotImplementedError{Command: "create"}, ExitError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCodeFor(tt.err); got != tt.want {
				t.Errorf("exitCodeFor(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

type wrapErr struct{ err error }

func (w wrapErr) Error() string { return "wrapped: " + w.err.Error() }
func (w wrapErr) Unwrap() error { return w.err }

func errWrap(e error) error { return wrapErr{err: e} }

func TestNotImplementedError_Message(t *testing.T) {
	e := &NotImplementedError{Command: "exec"}
	if e.Error() != "exec: not implemented yet" {
		t.Errorf("unexpected message: %q", e.Error())
	}
}
