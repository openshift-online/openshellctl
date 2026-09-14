package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/openshift-online/openshellctl/pkg/auth"
)

// recordingSource is a TokenSource that counts Invalidate calls.
type recordingSource struct{ invalidations int }

func (s *recordingSource) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{Source: auth.SourceClientCredentials, AccessToken: "t"}, nil
}
func (s *recordingSource) Invalidate()      { s.invalidations++ }
func (s *recordingSource) Describe() string { return "recording" }

func TestWithAuthRetry_Success(t *testing.T) {
	src := &recordingSource{}
	calls := 0
	err := withAuthRetry(context.Background(), src, func(context.Context) error {
		calls++
		return nil
	})
	if err != nil || calls != 1 || src.invalidations != 0 {
		t.Errorf("calls=%d invalidations=%d err=%v", calls, src.invalidations, err)
	}
}

func TestWithAuthRetry_RetriesOnceOnUnauthenticated(t *testing.T) {
	src := &recordingSource{}
	calls := 0
	err := withAuthRetry(context.Background(), src, func(context.Context) error {
		calls++
		if calls == 1 {
			return &UnauthenticatedError{Message: "expired"}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("err = %v, want nil after retry", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
	if src.invalidations != 1 {
		t.Errorf("invalidations = %d, want 1", src.invalidations)
	}
}

func TestWithAuthRetry_RetriesAtMostOnce(t *testing.T) {
	src := &recordingSource{}
	calls := 0
	err := withAuthRetry(context.Background(), src, func(context.Context) error {
		calls++
		return &UnauthenticatedError{Message: "still expired"}
	})
	var unauth *UnauthenticatedError
	if !errors.As(err, &unauth) {
		t.Fatalf("err = %v, want UnauthenticatedError", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want exactly 2 (one retry)", calls)
	}
}

func TestWithAuthRetry_NeverOnOtherErrors(t *testing.T) {
	src := &recordingSource{}
	calls := 0
	err := withAuthRetry(context.Background(), src, func(context.Context) error {
		calls++
		return &NotFoundError{Resource: "sandbox", Name: "x"}
	})
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want NotFoundError", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on non-auth error)", calls)
	}
	if src.invalidations != 0 {
		t.Errorf("invalidations = %d, want 0", src.invalidations)
	}
}

func TestWithAuthRetry_NilSource(t *testing.T) {
	calls := 0
	err := withAuthRetry(context.Background(), nil, func(context.Context) error {
		calls++
		return &UnauthenticatedError{Message: "expired"}
	})
	var unauth *UnauthenticatedError
	if !errors.As(err, &unauth) {
		t.Fatalf("err = %v, want UnauthenticatedError", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry without a source)", calls)
	}
}
