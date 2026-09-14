package gateway

import (
	"context"
	"errors"

	"github.com/openshift-online/openshellctl/pkg/auth"
)

// withAuthRetry runs fn once; if it returns a *UnauthenticatedError and a token
// source is present, it invalidates the cached token and retries exactly once.
// It never retries on any other error, and never falls back to no auth. Applied
// to every unary SDK-backed method; streaming methods use it only for the
// initial stream open. fn must already return Classify-wrapped errors.
func withAuthRetry(ctx context.Context, src auth.TokenSource, fn func(ctx context.Context) error) error {
	err := fn(ctx)
	if err == nil || src == nil {
		return err
	}
	var unauth *UnauthenticatedError
	if !errors.As(err, &unauth) {
		return err
	}
	src.Invalidate()
	return fn(ctx)
}
