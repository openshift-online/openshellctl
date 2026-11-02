// Package auth implements the openshellctl token lifecycle: typed tokens,
// pluggable token sources (client-credentials exchange, on-disk CLI bundle,
// static), JWT inspection, and CLI-interop write-back. It is the only auth
// abstraction the gateway layer sees (spec §5.2).
package auth

import (
	"context"
	"math"
	"time"
)

// Source identifies where a Token came from. SourceNone is the no-auth sentinel
// (auth_mode none/plaintext/mtls): Token.AccessToken is empty and the gateway
// layer sends no authorization header (see §5.3).
type Source string

// Token source kinds. SourceNone is the no-auth sentinel.
const (
	SourceNone              Source = ""
	SourceClientCredentials Source = "client_credentials"
	SourceDisk              Source = "disk"
	SourceStatic            Source = "static"
)

// Token is a resolved bearer token plus decoded (unverified) metadata.
type Token struct {
	AccessToken string
	Expiry      time.Time // authoritative: exchange time + expires_in; else JWT exp; zero = unknown
	IssuedAt    time.Time // JWT iat if present, else exchange/read time

	Issuer   string
	ClientID string
	Subject  string
	Audience []string // JWT aud (string or array)
	Roles    []string // realm_access.roles (Keycloak) if present

	Source Source
}

// Age reports how long ago the token was issued.
func (t Token) Age(now time.Time) time.Duration {
	return now.Sub(t.IssuedAt)
}

// ExpiresIn reports the time until expiry; negative when already expired, and
// math.MaxInt64 when the expiry is unknown (zero Expiry).
func (t Token) ExpiresIn(now time.Time) time.Duration {
	if t.Expiry.IsZero() {
		return time.Duration(math.MaxInt64)
	}
	return t.Expiry.Sub(now)
}

// Expired reports whether the token is within leeway of, or past, its expiry.
// A zero Expiry (unknown) is never considered expired.
func (t Token) Expired(now time.Time, leeway time.Duration) bool {
	if t.Expiry.IsZero() {
		return false
	}
	return !now.Before(t.Expiry.Add(-leeway))
}

// TokenSource is the only auth abstraction the gateway layer sees.
type TokenSource interface {
	Token(ctx context.Context) (*Token, error)
	// Invalidate drops any cached token so the next Token() re-exchanges/re-reads.
	// Used by the Unauthenticated retry.
	Invalidate()
	// Describe returns the human string for `token show`
	// (e.g. "client_credentials via metadata.json (gateway=rosa)").
	Describe() string
}

// bearerClassifier is an optional interface a TokenSource can implement to
// declare whether it ever produces a bearer token. Types that embed
// *NoAuthSource inherit its implementation (false) automatically. Types that
// do not implement this interface are assumed to require bearer auth (safe
// default). This is deliberately unexported so it does not widen the
// TokenSource contract or break existing test fakes.
type bearerClassifier interface {
	requiresBearer() bool
}

// RequiresBearer reports whether src will ever produce a bearer token.
// It returns false for nil and for any source that declares itself as
// non-bearer (via the unexported bearerClassifier interface — currently
// *NoAuthSource and any type embedding it); true for all other sources.
func RequiresBearer(src TokenSource) bool {
	if src == nil {
		return false
	}
	if bc, ok := src.(bearerClassifier); ok {
		return bc.requiresBearer()
	}
	return true
}
