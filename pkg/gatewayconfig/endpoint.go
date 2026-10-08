package gatewayconfig

import (
	"fmt"
	"net/url"
	"strings"
)

// defaultPortFor is the implicit port for a scheme, stripped by
// NormalizeEndpoint when given explicitly — an endpoint naming the default
// port and one omitting it refer to the same gateway.
var defaultPortFor = map[string]string{
	"https": "443",
	"http":  "80",
}

// NormalizeEndpoint returns a canonical form of endpoint for equality
// comparison against another endpoint: lower-cased scheme and host, the
// scheme's default port stripped when given explicitly (443 for https, 80
// for http), and a trailing "/" trimmed from the path. The query and
// fragment, when present, are preserved as-is — they are a meaningful part
// of the URL, not incidental formatting, so two endpoints differing only
// there must NOT be treated as the same gateway.
//
// This exists because two spellings of the same gateway endpoint
// (https://host/ vs https://host:443, as happened when a service account's
// exported OPENSHELL_GATEWAY_ENDPOINT had a trailing slash while `gateway
// add`/the CronJobs registered the same host with an explicit :443) used to
// fail to match in FindByEndpoint, silently ignoring the registered
// gateway's auth mode, token file, and TLS material.
//
// A missing scheme defaults to https (matching EnsureScheme's own default),
// consistent with `gateway add` only ever registering https endpoints.
// Returns an error only when endpoint cannot be parsed as a URL at all
// (empty, or a malformed host/port) — callers matching against a list of
// already-registered endpoints should treat that as "doesn't match
// anything" rather than letting one malformed entry abort the whole scan
// (see FindByEndpoint).
func NormalizeEndpoint(endpoint string) (string, error) {
	if endpoint == "" {
		return "", &InvalidEndpointError{Endpoint: endpoint, Cause: fmt.Errorf("empty endpoint")}
	}
	raw := endpoint
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", &InvalidEndpointError{Endpoint: endpoint, Cause: err}
	}
	if u.Hostname() == "" {
		return "", &InvalidEndpointError{Endpoint: endpoint, Cause: fmt.Errorf("no host in endpoint")}
	}

	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]" // re-bracket an IPv6 literal; Hostname() strips the brackets
	}
	port := u.Port()
	if port != "" && port != defaultPortFor[scheme] {
		host += ":" + port
	}

	path := strings.TrimRight(u.Path, "/")

	var b strings.Builder
	b.WriteString(scheme)
	b.WriteString("://")
	b.WriteString(host)
	b.WriteString(path)
	if u.RawQuery != "" {
		b.WriteString("?")
		b.WriteString(u.RawQuery)
	}
	if u.Fragment != "" {
		b.WriteString("#")
		b.WriteString(u.Fragment)
	}
	return b.String(), nil
}
