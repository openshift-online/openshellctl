package auth

import (
	"context"
	"time"
)

// StaticSource wraps a fixed bearer token (from --token / OPENSHELL_TOKEN). Its
// Expiry and metadata come from the JWT's own claims when the token is a JWT;
// otherwise Expiry is zero (unknown) and the token is treated as non-expiring.
type StaticSource struct {
	token string
	clock func() time.Time
}

// NewStaticSource builds a static token source. clock defaults to time.Now.
func NewStaticSource(token string, clock func() time.Time) *StaticSource {
	if clock == nil {
		clock = time.Now
	}
	return &StaticSource{token: token, clock: clock}
}

// Token returns the static token, decorated with any JWT claims it carries. A
// static token that is a JWT past its exp returns *ErrTokenExpired (no hint
// about a client secret, since none applies to a --token input).
func (s *StaticSource) Token(ctx context.Context) (*Token, error) {
	now := s.clock()
	tok := &Token{
		AccessToken: s.token,
		Source:      SourceStatic,
		IssuedAt:    now,
	}
	if c, err := Inspect(s.token); err == nil {
		applyClaims(tok, c, now)
	}
	if tok.Expired(now, 0) {
		return nil, &ErrTokenExpired{Expiry: tok.Expiry, Age: tok.Age(now)}
	}
	return tok, nil
}

// Invalidate is a no-op for a static token.
func (s *StaticSource) Invalidate() {}

// Describe implements TokenSource.
func (s *StaticSource) Describe() string { return "static token (--token/OPENSHELL_TOKEN)" }

// applyClaims copies JWT claim fields into a Token. Expiry is taken from the
// JWT exp when present; IssuedAt from iat when present.
func applyClaims(tok *Token, c *Claims, now time.Time) {
	tok.Issuer = c.Iss
	tok.Subject = c.Sub
	tok.Audience = c.Aud
	tok.Roles = c.Roles
	if c.Exp > 0 {
		tok.Expiry = time.Unix(c.Exp, 0)
	}
	if c.Iat > 0 {
		tok.IssuedAt = time.Unix(c.Iat, 0)
	} else {
		tok.IssuedAt = now
	}
}
