package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openshift-online/openshellctl/pkg/auth"
	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
)

// bearerSource is a minimal non-no-auth TokenSource for dial tests.
type bearerSource struct{}

func (bearerSource) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{Source: auth.SourceStatic, AccessToken: "t"}, nil
}
func (bearerSource) Invalidate()      {}
func (bearerSource) Describe() string { return "bearer" }

func TestDial_PlaintextWithTLSMaterialRejected(t *testing.T) {
	_, err := Dial(DialConfig{
		Endpoint: "http://gw:8080",
		TLS:      gatewayconfig.TLSMaterial{Present: true, CAFile: "mtls/ca.crt"},
	})
	if !errors.Is(err, ErrPlaintextWithTLSMaterial) {
		t.Fatalf("err = %v, want ErrPlaintextWithTLSMaterial", err)
	}
}

func TestDial_PlaintextWithAuthRejected(t *testing.T) {
	_, err := Dial(DialConfig{
		Endpoint: "http://gw:8080",
		Auth:     bearerSource{},
	})
	if !errors.Is(err, ErrPlaintextWithAuth) {
		t.Fatalf("err = %v, want ErrPlaintextWithAuth", err)
	}
}

func TestDial_PlaintextNoAuthOK(t *testing.T) {
	// grpc.NewClient is lazy; no connection is attempted here.
	conn, err := Dial(DialConfig{
		Endpoint: "http://gw:8080",
		Auth:     auth.NewNoAuthSource("plaintext"),
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if conn.SDK == nil || conn.Raw == nil {
		t.Error("both SDK and Raw clients should be constructed")
	}
}

func TestDial_TLSBareEndpointOK(t *testing.T) {
	conn, err := Dial(DialConfig{
		Endpoint: "gw.example.com:443",
		Auth:     bearerSource{},
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
}

func TestPerRPC_NoAuthSendsNoHeader(t *testing.T) {
	p := &perRPCCredentials{src: auth.NewNoAuthSource("none")}
	md, err := p.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(md) != 0 {
		t.Errorf("no-auth should send no metadata, got %v", md)
	}
	if p.RequireTransportSecurity() {
		t.Error("no-auth should not require transport security")
	}
}

func TestPerRPC_BearerSendsHeader(t *testing.T) {
	p := &perRPCCredentials{src: bearerSource{}}
	md, err := p.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if md["authorization"] != "Bearer t" {
		t.Errorf("authorization = %q, want 'Bearer t'", md["authorization"])
	}
	if !p.RequireTransportSecurity() {
		t.Error("bearer should require transport security")
	}
}

func TestPerRPC_NilSource(t *testing.T) {
	p := &perRPCCredentials{}
	md, err := p.GetRequestMetadata(context.Background())
	if err != nil || md != nil {
		t.Errorf("nil source: md=%v err=%v", md, err)
	}
	if p.RequireTransportSecurity() {
		t.Error("nil source should not require transport security")
	}
}

// countingExchanger counts Exchange calls without doing real OIDC.
type countingExchanger struct{ calls int32 }

func (e *countingExchanger) Exchange(context.Context, auth.ClientCredentialsConfig) (string, time.Duration, error) {
	e.calls++
	return "t", time.Hour, nil
}

func TestDial_PlaintextClientCredentials_NoExchange(t *testing.T) {
	ex := &countingExchanger{}
	_, err := Dial(DialConfig{
		Endpoint: "http://gw.example:8080",
		Auth: auth.NewClientCredentialsSource(auth.ClientCredentialsConfig{
			Issuer:   "https://issuer",
			ClientID: "c",
			Secret:   func(context.Context) (string, error) { return "s", nil },
		}, ex),
	})
	if !errors.Is(err, ErrPlaintextWithAuth) {
		t.Fatalf("err = %v, want ErrPlaintextWithAuth", err)
	}
	if ex.calls != 0 {
		t.Errorf("exchanger called %d times, want 0 (decision must be static)", ex.calls)
	}
}

// fatalSource calls t.Fatal if Token is ever invoked. It verifies that
// RequireTransportSecurity decides statically, without fetching a token.
type fatalSource struct{ t *testing.T }

func (s *fatalSource) Token(context.Context) (*auth.Token, error) {
	s.t.Fatal("Token() must not be called by RequireTransportSecurity")
	return nil, nil
}
func (*fatalSource) Invalidate()      {}
func (*fatalSource) Describe() string { return "fatal" }

func TestPerRPC_RequireTransportSecurity_NoTokenFetch(t *testing.T) {
	p := &perRPCCredentials{src: &fatalSource{t: t}}
	if !p.RequireTransportSecurity() {
		t.Error("non-NoAuth source should require transport security")
	}
}
