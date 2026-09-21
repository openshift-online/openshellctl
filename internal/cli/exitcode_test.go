package cli

import (
	"errors"
	"testing"

	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/sandbox"
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

		// Gateway typed errors.
		{"gateway unauthenticated → auth", &gateway.UnauthenticatedError{Message: "no token"}, ExitAuth},
		{"gateway permission denied → auth", &gateway.PermissionDeniedError{Message: "role"}, ExitAuth},
		{"gateway not found → notfound", &gateway.NotFoundError{Resource: "sandbox", Name: "x"}, ExitNotFound},
		{"wrapped gateway not found → notfound", errWrap(&gateway.NotFoundError{Name: "x"}), ExitNotFound},
		{"gateway already exists → conflict", &gateway.AlreadyExistsError{Name: "x"}, ExitConflict},
		{"gateway conflict → conflict", &gateway.ConflictError{Message: "modified"}, ExitConflict},
		{"gateway invalid argument → usage", &gateway.InvalidArgumentError{Message: "name exceeds maximum length (20 > 19)"}, ExitUsage},
		{"wrapped gateway invalid argument → usage", errWrap(&gateway.InvalidArgumentError{Message: "bad"}), ExitUsage},

		// Sandbox provisioning / lifecycle errors.
		{"provision failed → provision", &sandbox.ErrProvisionFailed{Reason: "boom"}, ExitProvision},
		{"provision timeout → provision", &sandbox.ErrProvisionTimeout{}, ExitProvision},
		{"lifecycle error → provision", &sandbox.ErrLifecycle{Target: "Stopped"}, ExitProvision},
		{"lifecycle timeout → provision", &sandbox.ErrLifecycleTimeout{Target: "Ready"}, ExitProvision},
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
