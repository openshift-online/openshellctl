package auth

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestDescribeAndInvalidate(t *testing.T) {
	// StaticSource
	ss := NewStaticSource("t", nil)
	ss.Invalidate()
	if !strings.Contains(ss.Describe(), "static") {
		t.Errorf("static describe = %q", ss.Describe())
	}

	// NoAuthSource
	na := NewNoAuthSource("plaintext")
	na.Invalidate()
	if !strings.Contains(na.Describe(), "plaintext") {
		t.Errorf("noauth describe = %q", na.Describe())
	}
	if strings.Contains(NewNoAuthSource("").Describe(), "(") {
		t.Errorf("empty-reason noauth should have no parenthetical")
	}

	// DiskBundleSource
	db := NewDiskBundleSource(fstest.MapFS{}, nil, WithBundleGatewayName("rosa"))
	db.Invalidate()
	if !strings.Contains(db.Describe(), "rosa") {
		t.Errorf("disk describe = %q", db.Describe())
	}
	if strings.Contains(NewDiskBundleSource(fstest.MapFS{}, nil).Describe(), "gateway=") {
		t.Errorf("no-gateway disk describe should omit gateway=")
	}

	// ClientCredentialsSource Describe without gateway.
	cc := NewClientCredentialsSource(ccConfig(), &fakeExchanger{})
	if cc.Describe() != "client_credentials" {
		t.Errorf("cc describe = %q", cc.Describe())
	}
}

func TestWithBackoffApplied(t *testing.T) {
	cur := time.Unix(1000, 0)
	ex := &fakeExchanger{token: "t", expiresIn: time.Hour}
	src := NewClientCredentialsSource(ccConfig(), ex,
		WithClock(func() time.Time { return cur }),
		WithBackoff(2*time.Second, 8*time.Second))
	if src.backoffInitial != 2*time.Second || src.backoffMax != 8*time.Second {
		t.Errorf("backoff not applied: initial=%v max=%v", src.backoffInitial, src.backoffMax)
	}
}

func TestNoAuthTokenIsEmpty(t *testing.T) {
	tok, err := NewNoAuthSource("none").Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Source != SourceNone || tok.AccessToken != "" {
		t.Errorf("no-auth token = %+v", tok)
	}
}

func TestSDKExchanger_Constructor(t *testing.T) {
	if NewSDKExchanger() == nil {
		t.Fatal("NewSDKExchanger returned nil")
	}
}

func TestSDKExchanger_BadIssuerErrors(t *testing.T) {
	// A non-loopback http:// issuer must be rejected by the SDK's discovery
	// without any network call succeeding. We only assert that an error is
	// returned (the exact message is the SDK's).
	ex := NewSDKExchanger()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := ex.Exchange(ctx, ClientCredentialsConfig{
		Issuer:   "http://example.com",
		ClientID: "c",
		Secret:   func(context.Context) (string, error) { return "s", nil },
	})
	if err == nil {
		t.Error("expected an error for a non-loopback http:// issuer")
	}
}
