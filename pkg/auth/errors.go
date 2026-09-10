package auth

import (
	"errors"
	"fmt"
	"time"
)

// ErrNoExpiry is returned when an exchange yields a non-positive expires_in.
// The SDK's NewClientCredentialsAuth enforces the same invariant (spec §5.2).
var ErrNoExpiry = errors.New("token has no positive expiry (expires_in <= 0)")

// errBackoff is the cause used when a retry is suppressed by the failure backoff.
var errBackoff = errors.New("token exchange is in backoff after a recent failure")

// ErrTokenExpired reports an expired, unrecoverable token. Hint is a
// user-facing remediation string (empty for static tokens).
type ErrTokenExpired struct {
	Expiry time.Time
	Age    time.Duration
	Hint   string
}

func (e *ErrTokenExpired) Error() string {
	msg := fmt.Sprintf("token expired at %s", e.Expiry.UTC().Format(time.RFC3339))
	if e.Hint != "" {
		msg += "; " + e.Hint
	}
	return msg
}

// ErrBundleInvalid reports an oidc_token.json that cannot be used (e.g. missing
// issuer or client_id — the Rust loader silently returns None; we surface why).
type ErrBundleInvalid struct{ Reason string }

func (e *ErrBundleInvalid) Error() string {
	return "invalid oidc_token.json: " + e.Reason
}

// ExchangeError wraps a failed token exchange.
type ExchangeError struct{ Cause error }

func (e *ExchangeError) Error() string { return "token exchange failed: " + e.Cause.Error() }
func (e *ExchangeError) Unwrap() error { return e.Cause }

// ErrOIDCConfigMissing reports that no OIDC issuer could be resolved. Checked
// lists the sources that were consulted, for a helpful message.
type ErrOIDCConfigMissing struct{ Checked []string }

func (e *ErrOIDCConfigMissing) Error() string {
	msg := "OIDC issuer could not be resolved"
	if len(e.Checked) > 0 {
		msg += " (checked: "
		for i, c := range e.Checked {
			if i > 0 {
				msg += ", "
			}
			msg += c
		}
		msg += ")"
	}
	return msg
}

// ErrMTLSMaterialMissing reports that an mTLS gateway lacks the full cert triple.
type ErrMTLSMaterialMissing struct{ Gateway string }

func (e *ErrMTLSMaterialMissing) Error() string {
	return fmt.Sprintf("gateway %q uses mTLS but the client cert/key/CA triple is missing under mtls/", e.Gateway)
}

// ErrUnsupportedAuthMode reports an auth_mode openshellctl does not implement.
type ErrUnsupportedAuthMode struct{ Mode string }

func (e *ErrUnsupportedAuthMode) Error() string {
	return fmt.Sprintf("auth_mode %q is not supported by openshellctl", e.Mode)
}
