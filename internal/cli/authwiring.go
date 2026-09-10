package cli

import (
	"context"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/openshift-online/openshellctl/pkg/auth"
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

// resolveTokenSource builds a TokenSource from the current flags/env. It resolves
// the gateway (best-effort — endpoint-only is allowed) and then the auth source.
func resolveTokenSource(cmd *cobra.Command) (auth.TokenSource, *gatewayconfig.Target, error) {
	env, err := gatewayconfig.NewOSEnv()
	if err != nil {
		return nil, nil, err
	}

	name := viper.GetString("gateway")
	endpoint := viper.GetString("gateway-endpoint")

	var target *gatewayconfig.Target
	if name != "" || endpoint != "" {
		target, err = gatewayconfig.Resolve(env, gatewayconfig.ResolveInput{Endpoint: endpoint, Name: name})
	} else {
		target, err = gatewayconfig.Resolve(env, gatewayconfig.ResolveInput{})
	}
	if err != nil {
		return nil, nil, err
	}

	in := auth.ResolveInput{
		StaticToken:     viper.GetString("token"),
		ClientSecret:    clientSecretProvider(viper.GetString("client-secret-file")),
		Issuer:          viper.GetString("oidc-issuer"),
		ClientID:        viper.GetString("oidc-client-id"),
		Audience:        viper.GetString("oidc-audience"),
		Gateway:         target.Resolved,
		GatewayEndpoint: target.Endpoint,
	}
	if scopes := viper.GetString("oidc-scopes"); scopes != "" {
		in.Scopes = strings.Fields(scopes)
	}

	src, err := auth.Resolve(cmd.Context(), in, auth.NewSDKExchanger())
	if err != nil {
		return nil, target, err
	}
	return src, target, nil
}
