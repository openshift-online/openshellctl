package cli

import (
	"context"
	"encoding/json"
	"fmt"
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
func oidcConfigFetcher(ctx context.Context, endpoint string) (string, string, error) {
	url := strings.TrimSuffix(endpoint, "/") + "/auth/oidc-config"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
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
