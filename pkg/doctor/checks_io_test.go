package doctor

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestCheckDNS(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name       string
		host       string
		lookup     func(ctx context.Context, host string) ([]net.IP, error)
		wantStatus Status
		wantSub    string
	}{
		{
			name: "success",
			host: "gw.example.com",
			lookup: func(context.Context, string) ([]net.IP, error) {
				return []net.IP{net.ParseIP("10.0.0.1")}, nil
			},
			wantStatus: StatusPass,
			wantSub:    "10.0.0.1",
		},
		{
			name: "NXDOMAIN",
			host: "gw.invalid.example",
			lookup: func(context.Context, string) ([]net.IP, error) {
				return nil, &net.DNSError{Err: "no such host", Name: "gw.invalid.example", IsNotFound: true}
			},
			wantStatus: StatusFail,
			wantSub:    "no such host",
		},
		{
			name: "timeout",
			host: "gw.example.com",
			lookup: func(context.Context, string) ([]net.IP, error) {
				return nil, &net.DNSError{Err: "i/o timeout", Name: "gw.example.com", IsTimeout: true}
			},
			wantStatus: StatusFail,
			wantSub:    "timeout",
		},
		{
			name:       "no host to resolve",
			host:       "",
			lookup:     func(context.Context, string) ([]net.IP, error) { return nil, errors.New("should not be called") },
			wantStatus: StatusFail,
			wantSub:    "no host",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckDNS(ctx, tt.host, tt.lookup)
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v (detail: %s)", got.Status, tt.wantStatus, got.Detail)
			}
			if !strings.Contains(got.Detail, tt.wantSub) {
				t.Errorf("Detail = %q, want it to contain %q", got.Detail, tt.wantSub)
			}
			if tt.wantStatus == StatusFail && got.NextStep == "" {
				t.Error("expected a NextStep for a failed check")
			}
		})
	}
}

// httpJSONResponse builds an *http.Response with the given status and body,
// suitable for returning from a fake HTTPGet func (no real network/listener
// involved — CheckHTTP only reads resp.StatusCode/resp.Body).
func httpJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

func TestCheckHTTP(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name         string
		endpoint     string
		get          func(ctx context.Context, url string) (*http.Response, error)
		wantStatus   Status
		wantSub      string
		wantIssuer   string
		wantAudience string
	}{
		{
			name:     "success",
			endpoint: "https://gw.example.com",
			get: func(context.Context, string) (*http.Response, error) {
				return httpJSONResponse(200, `{"issuer":"https://issuer","audience":"openshell-cli"}`), nil
			},
			wantStatus:   StatusPass,
			wantSub:      "issuer=https://issuer",
			wantIssuer:   "https://issuer",
			wantAudience: "openshell-cli",
		},
		{
			name:     "connection error",
			endpoint: "https://nowhere.invalid",
			get: func(context.Context, string) (*http.Response, error) {
				return nil, errors.New("dial tcp: connection refused")
			},
			wantStatus: StatusFail,
			wantSub:    "connection refused",
		},
		{
			name:     "non-200",
			endpoint: "https://gw.example.com",
			get: func(context.Context, string) (*http.Response, error) {
				return httpJSONResponse(503, ""), nil
			},
			wantStatus: StatusFail,
			wantSub:    "503",
		},
		{
			name:     "malformed body",
			endpoint: "https://gw.example.com",
			get: func(context.Context, string) (*http.Response, error) {
				return httpJSONResponse(200, `not json`), nil
			},
			wantStatus: StatusFail,
			wantSub:    "malformed",
		},
		{
			name:       "no endpoint to check",
			endpoint:   "",
			get:        func(context.Context, string) (*http.Response, error) { return nil, errors.New("should not be called") },
			wantStatus: StatusFail,
			wantSub:    "no endpoint",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, oidcCfg := CheckHTTP(ctx, tt.endpoint, tt.get)
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v (detail: %s)", got.Status, tt.wantStatus, got.Detail)
			}
			if !strings.Contains(got.Detail, tt.wantSub) {
				t.Errorf("Detail = %q, want it to contain %q", got.Detail, tt.wantSub)
			}
			if tt.wantStatus == StatusFail && got.NextStep == "" {
				t.Error("expected a NextStep for a failed check")
			}
			if tt.wantStatus == StatusPass {
				if oidcCfg.Issuer != tt.wantIssuer || oidcCfg.Audience != tt.wantAudience {
					t.Errorf("OIDCConfigResult = %+v, want issuer=%q audience=%q", oidcCfg, tt.wantIssuer, tt.wantAudience)
				}
			}
		})
	}
}
