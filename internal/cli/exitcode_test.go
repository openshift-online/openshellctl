package cli

import (
	"errors"
	"testing"

	"github.com/openshift-online/openshellctl/pkg/auth"
	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
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
		{"gateway permission denied → forbidden", &gateway.PermissionDeniedError{Message: "role"}, ExitForbidden},
		{"gateway not found → notfound", &gateway.NotFoundError{Resource: "sandbox", Name: "x"}, ExitNotFound},
		{"wrapped gateway not found → notfound", errWrap(&gateway.NotFoundError{Name: "x"}), ExitNotFound},
		{"gateway already exists → conflict", &gateway.AlreadyExistsError{Name: "x"}, ExitConflict},
		{"gateway conflict → conflict", &gateway.ConflictError{Message: "modified"}, ExitConflict},
		{"gateway invalid argument → usage", &gateway.InvalidArgumentError{Message: "name exceeds maximum length (20 > 19)"}, ExitUsage},
		{"wrapped gateway invalid argument → usage", errWrap(&gateway.InvalidArgumentError{Message: "bad"}), ExitUsage},
		{"gateway unavailable → error", &gateway.UnavailableError{Message: "down"}, ExitError},
		{"gateway deadline exceeded → error", &gateway.DeadlineError{}, ExitError},
		{"gateway rpc error (catch-all) → error", &gateway.RPCError{Message: "huh"}, ExitError},

		// Sandbox provisioning / lifecycle errors.
		{"provision failed → provision", &sandbox.ErrProvisionFailed{Reason: "boom"}, ExitProvision},
		{"provision timeout → provision", &sandbox.ErrProvisionTimeout{}, ExitProvision},
		{"lifecycle error → provision", &sandbox.ErrLifecycle{Target: "Stopped"}, ExitProvision},
		{"lifecycle timeout → provision", &sandbox.ErrLifecycleTimeout{Target: "Ready"}, ExitProvision},
		{"delete timeout → provision", &sandbox.ErrDeleteTimeout{Name: "sb"}, ExitProvision},

		// gatewayconfig typed errors (gateway add/list/select/remove/logout).
		{"gateway exists → conflict", &gatewayconfig.GatewayExistsError{Name: "x"}, ExitConflict},
		{"edge gateway unsupported → usage", &gatewayconfig.EdgeGatewayUnsupportedError{Endpoint: "https://x"}, ExitUsage},
		{"mtls unsupported → usage", &gatewayconfig.MTLSUnsupportedError{}, ExitUsage},
		{"invalid endpoint → usage", &gatewayconfig.InvalidEndpointError{Endpoint: "x", Cause: errors.New("bad")}, ExitUsage},
		{"invalid gateway name → usage", &gatewayconfig.InvalidGatewayNameError{Name: "a/b"}, ExitUsage},
		{"metadata parse error → usage", &gatewayconfig.MetadataParseError{Name: "x", Cause: errors.New("bad json")}, ExitUsage},

		// pkg/auth typed errors.
		{"token expired → auth", &auth.ErrTokenExpired{}, ExitAuth},
		{"exchange error → auth", &auth.ExchangeError{Cause: errors.New("boom")}, ExitAuth},
		{"oidc config missing → auth", &auth.ErrOIDCConfigMissing{}, ExitAuth},
		{"bundle invalid → auth", &auth.ErrBundleInvalid{Reason: "bad"}, ExitAuth},
		{"mtls material missing → auth", &auth.ErrMTLSMaterialMissing{Gateway: "x"}, ExitAuth},
		{"unsupported auth mode → usage", &auth.ErrUnsupportedAuthMode{Mode: "cloudflare_jwt"}, ExitUsage},
		{"no expiry (sentinel) → auth", auth.ErrNoExpiry, ExitAuth},
		{"no credentials → auth", &auth.ErrNoCredentials{}, ExitAuth},
		{"wrapped no credentials → auth", errWrap(&auth.ErrNoCredentials{}), ExitAuth},
		{"nothing to refresh → auth", &auth.ErrNothingToRefresh{}, ExitAuth},
		{"wrapped nothing to refresh → auth", errWrap(&auth.ErrNothingToRefresh{}), ExitAuth},
		{"not a jwt (sentinel) → usage", auth.ErrNotJWT, ExitUsage},
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
