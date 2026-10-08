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
		{"gateway unauthenticated, no specific reason -> refresh hint", &gateway.UnauthenticatedError{Message: "x"}, false, "token refresh"},
		{"gateway unauthenticated, expired signature -> refresh hint", &gateway.UnauthenticatedError{Message: "invalid token: ExpiredSignature"}, false, "token refresh"},

		// ErrNoCredentials must NOT get the circular refresh hint — `token
		// refresh` resolves auth the same way every other command does, so
		// it would hit the exact same error again.
		{"no credentials -> distinct hint, not refresh", &auth.ErrNoCredentials{}, false, "login"},
		{"wrapped no credentials -> distinct hint, not refresh", errWrap(&auth.ErrNoCredentials{}), false, "login"},

		// A wrong audience/issuer is a configuration mismatch, not an expiry
		// — refreshing the same misconfigured token changes nothing, so each
		// gets its own hint naming the actual fix.
		{"gateway unauthenticated, invalid audience -> audience hint", &gateway.UnauthenticatedError{Message: "invalid token: InvalidAudience"}, false, "audience"},
		{"gateway unauthenticated, invalid issuer -> issuer hint", &gateway.UnauthenticatedError{Message: "invalid token: InvalidIssuer"}, false, "issuer"},

		// Permission-denied must NOT get the refresh hint — it's a
		// fundamentally different problem (authorization, not expiry).
		{"permission denied -> distinct hint, not refresh", &gateway.PermissionDeniedError{Message: "no role"}, false, "role"},

		// nothing-to-refresh carries its own complete explanation already.
		{"nothing to refresh -> no hint", &auth.ErrNothingToRefresh{}, true, ""},
		{"nothing to refresh wrapping no credentials -> still gets the no-credentials hint", &auth.ErrNothingToRefresh{Cause: &auth.ErrNoCredentials{}}, false, "login"},

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

// TestHintFor_PermissionDeniedNamesBothFixes confirms the permission-denied
// hint states the two facts that actually resolve this in practice (per PR
// review): a human account needs the openshell-user realm role, and a
// service account must use its own OIDC client ID rather than the shared
// openshell-cli default — not just a vague "ask an administrator".
func TestHintFor_PermissionDeniedNamesBothFixes(t *testing.T) {
	hint := hintFor(&gateway.PermissionDeniedError{Message: "missing role"})
	if !contains(hint, "openshell-user") {
		t.Errorf("hint should name the openshell-user role, got: %q", hint)
	}
	if !contains(hint, "openshell-cli") {
		t.Errorf("hint should mention the shared openshell-cli client ID, got: %q", hint)
	}
}

// TestHintFor_NoCredentialsNamesBothPaths confirms the ErrNoCredentials hint
// names the fix for both a service account (export the secret) and a human
// (browser login) — not just a repeat of the "try token refresh" advice that
// produced the exact same error in the first place.
func TestHintFor_NoCredentialsNamesBothPaths(t *testing.T) {
	hint := hintFor(&auth.ErrNoCredentials{})
	if !contains(hint, "OPENSHELL_OIDC_CLIENT_SECRET") {
		t.Errorf("hint should name the service-account path, got: %q", hint)
	}
	if !contains(hint, "openshellctl login") {
		t.Errorf("hint should name the human/browser-login path, got: %q", hint)
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
