package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// LookupIPFunc resolves a hostname to its addresses — net.DefaultResolver.
// LookupIPAddr-shaped so production code can wire the real resolver directly
// as a one-line adapter, while tests pass a fake that never touches the
// network.
type LookupIPFunc func(ctx context.Context, host string) ([]net.IP, error)

// HTTPGetFunc performs an HTTP GET — http.Client.Get-shaped (minus the bare
// string-vs-context split) so production code can wire a real *http.Client
// (sharing http.DefaultTransport, the same as internal/cli's
// oidcConfigFetcher, so --gateway-insecure is honored via that one existing
// global toggle rather than a second one), while tests pass a fake that
// never touches the network.
type HTTPGetFunc func(ctx context.Context, url string) (*http.Response, error)

// CheckDNS resolves host via lookupIP. host should be a bare hostname (no
// scheme/port) — callers extract it from the endpoint URL before calling.
func CheckDNS(ctx context.Context, host string, lookupIP LookupIPFunc) CheckResult {
	if host == "" {
		return CheckResult{
			Name: "DNS", Status: StatusFail, Detail: "no host to resolve",
			NextStep: "resolve the Endpoint URL check's failure first",
		}
	}
	ips, err := lookupIP(ctx, host)
	if err != nil {
		return CheckResult{
			Name:     "DNS",
			Status:   StatusFail,
			Detail:   fmt.Sprintf("could not resolve %q: %v", host, err),
			NextStep: fmt.Sprintf("check DNS/VPN connectivity for %q (try: nslookup %s)", host, host),
		}
	}
	return CheckResult{Name: "DNS", Status: StatusPass, Detail: fmt.Sprintf("%s -> %v", host, ips)}
}

// OIDCConfig is the result of a successful CheckHTTP discovery fetch,
// consumed by CheckOIDCConfigMatch (checks_pure.go).
type OIDCConfig struct {
	Issuer   string
	Audience string
}

// CheckHTTP fetches <endpoint>/auth/oidc-config via get, confirming the
// gateway is reachable over HTTPS and serves a well-formed discovery
// document. On success, also returns the discovered OIDCConfig for
// CheckOIDCConfigMatch to compare against the registered gateway's metadata
// — this check's job is "can I reach it and parse the response", not "does
// it match what's registered", which is why both pieces of data flow out
// rather than CheckHTTP making that comparison itself.
func CheckHTTP(ctx context.Context, endpoint string, get HTTPGetFunc) (CheckResult, OIDCConfig) {
	if endpoint == "" {
		return CheckResult{
			Name: "HTTP reachability", Status: StatusFail, Detail: "no endpoint to check",
			NextStep: "resolve the Endpoint URL check's failure first",
		}, OIDCConfig{}
	}
	u := strings.TrimSuffix(endpoint, "/") + "/auth/oidc-config"
	resp, err := get(ctx, u)
	if err != nil {
		return CheckResult{
			Name:     "HTTP reachability",
			Status:   StatusFail,
			Detail:   fmt.Sprintf("GET %s: %v", u, err),
			NextStep: fmt.Sprintf("check the gateway is reachable (try: curl -v %s), or pass --gateway-insecure for an internal CA", u),
		}, OIDCConfig{}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return CheckResult{
			Name:     "HTTP reachability",
			Status:   StatusFail,
			Detail:   fmt.Sprintf("GET %s returned %d %s", u, resp.StatusCode, resp.Status),
			NextStep: "check the gateway endpoint is correct and serves /auth/oidc-config",
		}, OIDCConfig{}
	}

	var body struct {
		Issuer   string `json:"issuer"`
		Audience string `json:"audience"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return CheckResult{
			Name:     "HTTP reachability",
			Status:   StatusFail,
			Detail:   fmt.Sprintf("malformed /auth/oidc-config response from %s: %v", u, err),
			NextStep: "check the gateway's /auth/oidc-config endpoint returns valid JSON",
		}, OIDCConfig{}
	}

	return CheckResult{
		Name:   "HTTP reachability",
		Status: StatusPass,
		Detail: fmt.Sprintf("%s reachable (issuer=%s)", u, body.Issuer),
	}, OIDCConfig{Issuer: body.Issuer, Audience: body.Audience}
}
