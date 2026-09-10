package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStaticSource_JWTToken(t *testing.T) {
	now := time.Unix(1000, 0)
	jwt := makeJWT(t, map[string]any{
		"iss": "https://i",
		"sub": "user-1",
		"aud": "openshell-cli",
		"exp": float64(2000),
		"iat": float64(500),
	})
	src := NewStaticSource(jwt, func() time.Time { return now })

	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.Source != SourceStatic {
		t.Errorf("Source = %v, want static", tok.Source)
	}
	if tok.Subject != "user-1" {
		t.Errorf("Subject = %q", tok.Subject)
	}
	if !tok.Expiry.Equal(time.Unix(2000, 0)) {
		t.Errorf("Expiry = %v, want 2000", tok.Expiry)
	}
	if !tok.IssuedAt.Equal(time.Unix(500, 0)) {
		t.Errorf("IssuedAt = %v, want 500", tok.IssuedAt)
	}
}

func TestStaticSource_ExpiredJWT(t *testing.T) {
	now := time.Unix(3000, 0)
	jwt := makeJWT(t, map[string]any{"iss": "https://i", "exp": float64(2000)})
	src := NewStaticSource(jwt, func() time.Time { return now })

	_, err := src.Token(context.Background())
	var expired *ErrTokenExpired
	if !errors.As(err, &expired) {
		t.Fatalf("err = %v, want ErrTokenExpired", err)
	}
	if expired.Hint != "" {
		t.Errorf("static expiry should carry no hint, got %q", expired.Hint)
	}
}

func TestStaticSource_OpaqueToken(t *testing.T) {
	now := time.Unix(1000, 0)
	src := NewStaticSource("opaque-not-a-jwt", func() time.Time { return now })

	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "opaque-not-a-jwt" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}
	if !tok.Expiry.IsZero() {
		t.Errorf("opaque token should have zero (unknown) Expiry, got %v", tok.Expiry)
	}
}
