package cli

import (
	"errors"
	"testing"

	"github.com/openshift-online/openshellctl/pkg/auth"
	"github.com/openshift-online/openshellctl/pkg/gateway"
)

func TestHintFor(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantEmpty bool
		wantSub   string
	}{
		{"nil -> no hint", nil, true, ""},
		{"generic error -> no hint", errors.New("boom"), true, ""},
		{"usage error -> no hint", &UsageError{Err: errors.New("bad")}, true, ""},

		// The refresh hint applies to every "your token/auth is stale or
		// missing" case.
		{"token expired -> refresh hint", &auth.ErrTokenExpired{}, false, "token refresh"},
		{"exchange error -> refresh hint", &auth.ExchangeError{Cause: errors.New("x")}, false, "token refresh"},
		{"oidc config missing -> refresh hint", &auth.ErrOIDCConfigMissing{}, false, "token refresh"},
		{"gateway unauthenticated -> refresh hint", &gateway.UnauthenticatedError{Message: "x"}, false, "token refresh"},
		{"no credentials -> refresh hint", auth.ErrNoCredentials, false, "token refresh"},
		{"wrapped no credentials -> refresh hint", errWrap(auth.ErrNoCredentials), false, "token refresh"},

		// Permission-denied must NOT get the refresh hint — it's a
		// fundamentally different problem (authorization, not expiry).
		{"permission denied -> distinct hint, not refresh", &gateway.PermissionDeniedError{Message: "no role"}, false, "role"},

		// nothing-to-refresh carries its own complete explanation already.
		{"nothing to refresh -> no hint", &auth.ErrNothingToRefresh{}, true, ""},

		// Every other exported error type in pkg/auth and pkg/gateway gets no
		// hint (per the ticket's acceptance criterion: a test row for every
		// exported error type in both packages).
		{"bundle invalid -> no hint", &auth.ErrBundleInvalid{Reason: "x"}, true, ""},
		{"mtls material missing -> no hint", &auth.ErrMTLSMaterialMissing{Gateway: "x"}, true, ""},
		{"unsupported auth mode -> no hint", &auth.ErrUnsupportedAuthMode{Mode: "x"}, true, ""},
		{"no expiry -> no hint", auth.ErrNoExpiry, true, ""},
		{"not a jwt -> no hint", auth.ErrNotJWT, true, ""},
		{"not found -> no hint", &gateway.NotFoundError{Name: "x"}, true, ""},
		{"already exists -> no hint", &gateway.AlreadyExistsError{Name: "x"}, true, ""},
		{"conflict -> no hint", &gateway.ConflictError{Message: "x"}, true, ""},
		{"invalid argument -> no hint", &gateway.InvalidArgumentError{Message: "x"}, true, ""},
		{"unavailable -> no hint", &gateway.UnavailableError{Message: "x"}, true, ""},
		{"deadline exceeded -> no hint", &gateway.DeadlineError{}, true, ""},
		{"rpc error (catch-all) -> no hint", &gateway.RPCError{Message: "x"}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hintFor(tt.err)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("hintFor(%v) = %q, want empty", tt.err, got)
				}
				return
			}
			if got == "" {
				t.Fatalf("hintFor(%v) = empty, want a hint containing %q", tt.err, tt.wantSub)
			}
			if !contains(got, tt.wantSub) {
				t.Errorf("hintFor(%v) = %q, want it to contain %q", tt.err, got, tt.wantSub)
			}
		})
	}
}

// TestHintFor_PermissionDeniedDoesNotSuggestRefresh pins the exact bug being
// fixed: a permission-denied response must never get the same hint as an
// expired token, since re-running `token refresh` cannot fix an
// authorization problem.
func TestHintFor_PermissionDeniedDoesNotSuggestRefresh(t *testing.T) {
	hint := hintFor(&gateway.PermissionDeniedError{Message: "missing role"})
	if contains(hint, "token refresh") {
		t.Errorf("permission-denied hint must not suggest token refresh, got: %q", hint)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
