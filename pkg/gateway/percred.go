package gateway

import (
	"context"

	"github.com/openshift-online/openshellctl/pkg/auth"
)

// perRPCCredentials adapts an auth.TokenSource to grpc/credentials.PerRPCCredentials
// (and the SDK's types.AuthProvider, which is the same shape). Transport security
// is decided statically from the source type (not by fetching a token). It sends
// "authorization: Bearer <jwt>" unless the source is a no-auth source, in which
// case it sends no header.
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

// requiresTransportSecurity reports whether per-RPC creds demand TLS. Decided
// statically from the source type, never by fetching a token.
func (p *perRPCCredentials) requiresTransportSecurity() bool {
	return auth.RequiresBearer(p.src)
}

// hasAuth reports whether a non-no-auth token source is present (used to decide
// whether to attach per-RPC creds to the raw conn).
func (p *perRPCCredentials) hasAuth() bool {
	return p.requiresTransportSecurity()
}
