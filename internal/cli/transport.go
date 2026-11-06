package cli

import "net/http"

// cleanTransport is captured at package-init time, before PersistentPreRunE
// can call applyGatewayInsecureTransport and mutate http.DefaultTransport.
// Used by update/notice HTTP clients, the OIDC-config discovery fetcher, and
// the client-credentials fallback in resolveTokenSource — any client that must
// verify TLS regardless of --gateway-insecure.
var cleanTransport = func() *http.Transport {
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		return base.Clone()
	}
	return &http.Transport{Proxy: http.ProxyFromEnvironment}
}()

// freshTransport returns a clone of the transport captured before
// applyGatewayInsecureTransport can modify http.DefaultTransport.
func freshTransport() http.RoundTripper {
	return cleanTransport.Clone()
}
