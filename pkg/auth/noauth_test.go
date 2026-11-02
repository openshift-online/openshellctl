package auth

import (
	"context"
	"testing"
	"testing/fstest"
	"time"
)

func TestRequiresBearer(t *testing.T) {
	countingEx := &fakeExchanger{token: "tok", expiresIn: time.Hour}

	tests := []struct {
		name string
		src  TokenSource
		want bool
	}{
		{name: "nil", src: nil, want: false},
		{name: "NoAuthSource/mtls", src: NewNoAuthSource("mtls"), want: false},
		{name: "NoAuthSource/none", src: NewNoAuthSource("none"), want: false},
		{name: "StaticSource", src: NewStaticSource("x", nil), want: true},
		{name: "ClientCredentialsSource", src: NewClientCredentialsSource(ccConfig(), countingEx), want: true},
		{name: "DiskBundleSource", src: NewDiskBundleSource(fstest.MapFS{}, nil), want: true},
	}

	// Wrapper that embeds *NoAuthSource — must inherit requiresBearer() and
	// be classified as non-bearer, not silently treated as bearer.
	type wrappedNoAuth struct{ *NoAuthSource }
	tests = append(tests, struct {
		name string
		src  TokenSource
		want bool
	}{name: "embedded NoAuthSource wrapper", src: wrappedNoAuth{NewNoAuthSource("wrapped")}, want: false})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RequiresBearer(tt.src)
			if got != tt.want {
				t.Errorf("RequiresBearer(%T) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}

	if countingEx.count() != 0 {
		t.Errorf("exchanger was called %d times, want 0 (RequiresBearer must not fetch a token)", countingEx.count())
	}
}

// fatalTokenSource is a TokenSource whose Token method calls t.Fatal.
// It verifies that RequiresBearer never calls Token.
type fatalTokenSource struct{ t *testing.T }

func (s *fatalTokenSource) Token(context.Context) (*Token, error) {
	s.t.Fatal("Token() must not be called by RequiresBearer")
	return nil, nil
}
func (*fatalTokenSource) Invalidate()      {}
func (*fatalTokenSource) Describe() string { return "fatal" }

func TestRequiresBearer_DoesNotCallToken(t *testing.T) {
	src := &fatalTokenSource{t: t}
	got := RequiresBearer(src)
	if !got {
		t.Error("non-NoAuthSource should return true")
	}
}
