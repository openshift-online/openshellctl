package gateway

import (
	"context"

	"github.com/openshift-online/openshellctl/pkg/auth"
)

// perRPCCredentials adapts an auth.TokenSource to grpc/credentials.PerRPCCredentials
// (and the SDK's types.AuthProvider, which is the same shape). It sends
// "authorization: Bearer <jwt>" unless the token is a no-auth (SourceNone)
// token, in which case it sends no header and does not require transport
// security.
type perRPCCredentials struct{ src auth.TokenSource }

// GetRequestMetadata implements credentials.PerRPCCredentials / types.AuthProvider.
func (p *perRPCCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	if p.src == nil {
		return nil, nil
	}
	tok, err := p.src.Token(ctx)
	if err != nil {
		return nil, err
	}
	if tok.Source == auth.SourceNone || tok.AccessToken == "" {
		return nil, nil
	}
	return map[string]string{"authorization": "Bearer " + tok.AccessToken}, nil
}

// RequireTransportSecurity returns true unless the source is a no-auth source.
func (p *perRPCCredentials) RequireTransportSecurity() bool {
	return p.requiresTransportSecurity()
}

// requiresTransportSecurity reports whether per-RPC creds demand TLS. It is
// false when there is no source, or when the resolved token is a no-auth token.
// A resolution error is treated as "requires security" (fail safe).
func (p *perRPCCredentials) requiresTransportSecurity() bool {
	if p.src == nil {
		return false
	}
	tok, err := p.src.Token(context.Background())
	if err != nil {
		return true
	}
	return tok.Source != auth.SourceNone
}

// hasAuth reports whether a non-no-auth token source is present (used to decide
// whether to attach per-RPC creds to the raw conn).
func (p *perRPCCredentials) hasAuth() bool {
	return p.requiresTransportSecurity()
}
