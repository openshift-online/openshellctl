package cli

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/openshift-online/openshellctl/internal/version"
	"github.com/openshift-online/openshellctl/pkg/auth"
	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
)

// clientSecretProvider returns a secret-reading closure, or nil when no secret
// source is configured. Precedence: OPENSHELL_OIDC_CLIENT_SECRET env (via viper)
// > --client-secret-file. The env value is never logged.
func clientSecretProvider(clientSecretFile string) func(ctx context.Context) (string, error) {
	if secret := viper.GetString("oidc.client_secret"); secret != "" {
		return func(context.Context) (string, error) { return secret, nil }
	}
	if clientSecretFile != "" {
		return func(context.Context) (string, error) {
			b, err := os.ReadFile(clientSecretFile)
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(string(b)), nil
		}
	}
	return nil
}

// authWriterAdapter adapts a *gatewayconfig.OSWriter to auth.Writer, rooting
// writes at the gateway's directory so a bare "oidc_token.json" relPath lands in
// gateways/<name>/oidc_token.json on the user tree.
type authWriterAdapter struct {
	w           *gatewayconfig.OSWriter
	gatewayName string
}

func (a authWriterAdapter) WriteFile(relPath string, data []byte, perm fs.FileMode) error {
	return a.w.WriteFile(gatewayconfig.GatewayDir(a.gatewayName)+"/"+relPath, data, perm)
}

// tokenWriterFor builds an auth.Writer that persists to the resolved gateway's
// directory on the user tree, or (nil, nil) when there is no named gateway to
// write to (e.g. endpoint-only invocation).
func tokenWriterFor(target *gatewayconfig.Target) (auth.Writer, error) {
	if target == nil || target.Name == "" || target.Resolved == nil {
		return nil, nil
	}
	w, err := gatewayconfig.NewOSWriter()
	if err != nil {
		return nil, err
	}
	return authWriterAdapter{w: w, gatewayName: target.Name}, nil
}

// oidcConfigFetcher fetches {issuer, audience} from GET <endpoint>/auth/oidc-config.
// Honors --gateway-insecure the same way the real gRPC dial does (see
// pkg/gateway/dial.go's Insecure field): a staging/internal gateway whose
// certificate isn't in the public trust store would otherwise make `gateway
// add` discovery fail even when the gateway is perfectly reachable — and
// since no gateway is registered yet at this point, there's no per-gateway
// CA bundle (gatewayconfig's mtls/ca.crt) this call could use instead.
func oidcConfigFetcher(ctx context.Context, endpoint string) (string, string, error) {
	url := strings.TrimSuffix(endpoint, "/") + "/auth/oidc-config"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	if viper.GetBool("gateway-insecure") {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // explicit opt-in via --gateway-insecure, mirroring pkg/gateway/dial.go
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("/auth/oidc-config returned %s", resp.Status)
	}
	var body struct {
		Issuer   string `json:"issuer"`
		Audience string `json:"audience"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", err
	}
	return body.Issuer, body.Audience, nil
}

// resolveTokenSource builds a TokenSource from the current flags/env. It resolves
// the gateway (best-effort — endpoint-only is allowed) and then the auth source.
// When --write-token is set and a gateway is resolved, a write-back Writer is
// wired so a disk-bundle refresh persists (Rust CLI schema).
func resolveTokenSource(cmd *cobra.Command) (auth.TokenSource, *gatewayconfig.Target, error) {
	env, err := gatewayconfig.NewOSEnv()
	if err != nil {
		return nil, nil, err
	}

	name := viper.GetString("gateway")
	endpoint := viper.GetString("gateway-endpoint")

	target, err := gatewayconfig.Resolve(env, gatewayconfig.ResolveInput{Endpoint: endpoint, Name: name})
	if err != nil {
		return nil, nil, err
	}

	in := auth.ResolveInput{
		StaticToken:       viper.GetString("token"),
		ClientSecret:      clientSecretProvider(viper.GetString("client-secret-file")),
		Issuer:            viper.GetString("oidc-issuer"),
		ClientID:          viper.GetString("oidc-client-id"),
		Audience:          viper.GetString("oidc-audience"),
		Gateway:           target.Resolved,
		GatewayEndpoint:   target.Endpoint,
		OIDCConfigFetcher: oidcConfigFetcher,
	}
	if scopes := viper.GetString("oidc-scopes"); scopes != "" {
		in.Scopes = strings.Fields(scopes)
	}

	// Wire write-back when requested and a named gateway is resolved.
	if viper.GetBool("write-token") {
		if w, werr := tokenWriterFor(target); werr == nil && w != nil {
			in.TokenWriter = w
		}
	}

	src, err := auth.Resolve(cmd.Context(), in, auth.NewSDKExchanger())
	if err != nil {
		return nil, target, err
	}
	return src, target, nil
}

// injectedTarget is the Target substituted for a nil deps.Target on the
// injected path, so callers that read target.Name/target.Endpoint without a
// nil-guard get a well-defined test value instead of a nil-pointer panic.
// Resolved stays nil, matching the normal "endpoint-only" state elsewhere in
// this package.
var injectedTarget = &gatewayconfig.Target{Name: "test", Endpoint: "https://test.invalid"}

// resolveAuth is resolveTokenSource, but honors cliDeps (deps.go) injected on
// the command's context: if either Gateway or TokenSource was injected, it
// returns the injected TokenSource (or, when only Gateway was injected, a
// auth.NoAuthSource stand-in — never nil, so a caller that calls src.Token()
// doesn't panic) and the injected Target (defaulted via injectedTarget when
// nil), skipping the real resolveTokenSource entirely. Checking Gateway here
// too (not just TokenSource) keeps this in lockstep with dialOrInjected/
// withGatewayTarget, which key off the same two fields — see the package doc
// on cliDeps for why a single command must never see one seam honor an
// injected dep that another seam on the same call ignores.
//
// Production code never sets cliDeps, so this always falls through to the
// real resolveTokenSource there.
func resolveAuth(cmd *cobra.Command) (auth.TokenSource, *gatewayconfig.Target, error) {
	deps, ok := depsFrom(cmd.Context())
	if !ok || (deps.TokenSource == nil && deps.Gateway == nil) {
		return resolveTokenSource(cmd)
	}
	target := deps.Target
	if target == nil {
		target = injectedTarget
	}
	src := deps.TokenSource
	if src == nil {
		src = auth.NewNoAuthSource("test gateway injected, no token source injected")
	}
	return src, target, nil
}

// noopCloser is an io.Closer that does nothing, used by dialOrInjected to
// stand in for the real *gateway.Conn when the gateway was test-injected and
// there is nothing to close.
type noopCloser struct{}

func (noopCloser) Close() error { return nil }

// dialOrInjected is dialGateway, but returns a Gateway injected via cliDeps
// (deps.go) on the command's context when present, skipping the real dial.
// The returned io.Closer is a no-op in that case; *gateway.Conn (dialGateway's
// real return) already satisfies io.Closer, so callers need no other change.
func dialOrInjected(cmd *cobra.Command, target *gatewayconfig.Target, src auth.TokenSource) (gateway.Gateway, io.Closer, error) {
	if deps, ok := depsFrom(cmd.Context()); ok && deps.Gateway != nil {
		return deps.Gateway, noopCloser{}, nil
	}
	return dialGateway(target, src)
}

// dialGateway builds a gateway.Gateway from a resolved target and token source.
// The caller is responsible for closing the returned Conn.
func dialGateway(target *gatewayconfig.Target, src auth.TokenSource) (gateway.Gateway, *gateway.Conn, error) {
	dc := gateway.DialConfig{
		Endpoint:  target.Endpoint,
		Auth:      src,
		Insecure:  viper.GetBool("gateway-insecure"),
		UserAgent: userAgent(),
	}
	if target.Resolved != nil {
		dc.TLS = gatewayconfig.TLSMaterialFor(target.Resolved)
		dc.TLSRoot = target.Resolved.Dir
	}
	conn, err := gateway.Dial(dc)
	if err != nil {
		return nil, nil, err
	}
	return gateway.New(conn, src), conn, nil
}

func userAgent() string {
	v := version.Get()
	return fmt.Sprintf("openshellctl/%s (openshell-pin %s)", v.Version, v.OpenShellPin)
}
