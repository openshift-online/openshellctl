package auth

import "context"

// NoAuthSource represents auth_mode none/plaintext/mtls: it returns a Token with
// an empty AccessToken and Source SourceNone. The gateway layer sends no
// authorization header for such tokens (spec §5.2 rule 4/5).
type NoAuthSource struct{ reason string }

// NewNoAuthSource builds a no-auth source. reason describes why (for Describe).
func NewNoAuthSource(reason string) *NoAuthSource { return &NoAuthSource{reason: reason} }

// Token returns the empty no-auth token.
func (s *NoAuthSource) Token(context.Context) (*Token, error) {
	return &Token{Source: SourceNone}, nil
}

// Invalidate is a no-op.
func (s *NoAuthSource) Invalidate() {}

// Describe implements TokenSource.
func (s *NoAuthSource) Describe() string {
	if s.reason != "" {
		return "no auth (" + s.reason + ")"
	}
	return "no auth"
}
