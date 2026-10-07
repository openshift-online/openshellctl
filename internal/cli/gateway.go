package cli

import (
	"context"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/openshift-online/openshellctl/pkg/auth"
	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
	"github.com/openshift-online/openshellctl/pkg/output"
)

// newGatewayCommand builds `gateway` (alias `gw`) and its subcommands.
func newGatewayCommand() *cobra.Command {
	g := &cobra.Command{
		Use:     "gateway",
		Aliases: []string{"gw"},
		Short:   "Manage gateway registrations",
	}
	g.AddCommand(
		newGatewayAddCommand(),
		newGatewayListCommand(),
		newGatewaySelectCommand(),
		newGatewayRemoveCommand(),
		newGatewayLogoutCommand(),
		newGatewayLoginCommand(),
	)
	return g
}

// newGatewayLoginCommand is a thin alias of the root login command, under
// `gateway` for discoverability. A *cobra.Command can't be added to two
// parents, so this is a separate instance sharing runLogin (login.go).
func newGatewayLoginCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in to a gateway via OIDC browser flow",
		Long:  "Open a browser for OIDC authentication and persist the token to disk.",
		Args:  cobra.NoArgs,
		RunE:  runLogin,
	}
}

// newGatewayListCommand builds `gateway list`.
func newGatewayListCommand() *cobra.Command {
	var format string
	c := &cobra.Command{
		Use:   "list",
		Short: "List registered gateways",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := gatewayconfig.NewOSEnv()
			if err != nil {
				return err
			}
			gateways, err := gatewayconfig.ListDetailed(env)
			if err != nil {
				return err
			}
			return output.RenderGatewayList(cmd.OutOrStdout(), gateways, output.Format(format))
		},
	}
	c.Flags().StringVarP(&format, "output", "o", "table", "output format: table|json|yaml")
	return c
}

// newGatewaySelectCommand builds `gateway select <name>`.
func newGatewaySelectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "select <name>",
		Short: "Set the active gateway",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			env, err := gatewayconfig.NewOSEnv()
			if err != nil {
				return err
			}
			if _, err := gatewayconfig.Load(env, name); err != nil {
				return err
			}
			w, err := gatewayconfig.NewOSWriter()
			if err != nil {
				return err
			}
			if err := gatewayconfig.SetActive(w, name); err != nil {
				return err
			}
			cmd.Printf("✓ Gateway '%s' is now active\n", name)
			return nil
		},
	}
}

// newGatewayRemoveCommand builds `gateway remove <name>`.
func newGatewayRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a gateway registration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			env, err := gatewayconfig.NewOSEnv()
			if err != nil {
				return err
			}
			exists, err := gatewayconfig.Exists(env, name)
			if err != nil {
				return err
			}
			if !exists {
				return &gatewayconfig.GatewayNotFoundError{Name: name}
			}
			w, err := gatewayconfig.NewOSWriter()
			if err != nil {
				return err
			}
			if err := gatewayconfig.RemoveGateway(w, name); err != nil {
				return err
			}
			if err := gatewayconfig.ClearActiveIfMatches(w, name); err != nil {
				return err
			}
			cmd.Printf("✓ Gateway '%s' removed\n", name)
			return nil
		},
	}
}

// newGatewayLogoutCommand builds `gateway logout` — defaults to the active
// gateway (or -g/--gateway) like login does, via the same Resolve call.
func newGatewayLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the cached token for a gateway, keeping its registration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := gatewayconfig.NewOSEnv()
			if err != nil {
				return err
			}
			target, err := gatewayconfig.Resolve(env, gatewayconfig.ResolveInput{
				Endpoint: viper.GetString("gateway-endpoint"),
				Name:     viper.GetString("gateway"),
			})
			if err != nil {
				return err
			}
			w, err := gatewayconfig.NewOSWriter()
			if err != nil {
				return err
			}
			if err := gatewayconfig.Logout(w, target.Name); err != nil {
				return err
			}
			cmd.Printf("✓ Logged out of gateway '%s'\n", target.Name)
			return nil
		},
	}
}

// newGatewayAddCommand builds `gateway add <endpoint>`. It only registers
// OIDC gateways — mTLS and non-OIDC ("edge") gateways are out of scope for
// this epic (ROSAENG-68825) and return typed errors from NewMetadata.
func newGatewayAddCommand() *cobra.Command {
	var name string
	c := &cobra.Command{
		Use:   "add <endpoint>",
		Short: "Register an OIDC gateway",
		Long: "Register a gateway's metadata.json without the upstream Rust binary. " +
			"Without --oidc-issuer, the issuer/audience are discovered from " +
			"<endpoint>/auth/oidc-config; a gateway that doesn't answer that " +
			"endpoint is not supported (pass --oidc-issuer explicitly if it is " +
			"actually OIDC-configured).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGatewayAdd(cmd, args[0], name)
		},
	}
	c.Flags().StringVar(&name, "name", "", "gateway name (default: derived from the endpoint host)")
	return c
}

// runGatewayAdd normalizes the endpoint, discovers or takes an explicit OIDC
// issuer, builds and writes metadata.json, sets the new gateway active, and
// then attempts to authenticate it (see authenticateNewGateway).
func runGatewayAdd(cmd *cobra.Command, rawEndpoint, name string) error {
	endpoint, err := gatewayconfig.NormalizeEndpoint(rawEndpoint)
	if err != nil {
		return err
	}

	issuerOverride := viper.GetString("oidc-issuer")
	var discovered *gatewayconfig.DiscoveredOIDC
	if issuerOverride == "" {
		if iss, aud, derr := oidcConfigFetcher(cmd.Context(), endpoint); derr == nil {
			discovered = &gatewayconfig.DiscoveredOIDC{Issuer: iss, Audience: aud}
		}
		// A failed probe leaves discovered nil; NewMetadata below surfaces
		// EdgeGatewayUnsupportedError when neither an override nor a
		// successful discovery is available — the probe's own error (DNS,
		// timeout, non-200) isn't itself returned, since "not OIDC" and
		// "unreachable" get the same remediation: pass --oidc-issuer.
	}

	m, err := gatewayconfig.NewMetadata(gatewayconfig.AddInput{
		Endpoint:     endpoint,
		Name:         name,
		OIDCIssuer:   issuerOverride,
		OIDCClientID: viper.GetString("oidc-client-id"),
		OIDCAudience: viper.GetString("oidc-audience"),
		OIDCScopes:   viper.GetString("oidc-scopes"),
		Discovered:   discovered,
	})
	if err != nil {
		return err
	}

	env, err := gatewayconfig.NewOSEnv()
	if err != nil {
		return err
	}
	w, err := gatewayconfig.NewOSWriter()
	if err != nil {
		return err
	}
	if err := gatewayconfig.WriteGateway(w, env, m.Name, m); err != nil {
		return err
	}
	if err := gatewayconfig.SetActive(w, m.Name); err != nil {
		return err
	}
	cmd.Printf("✓ Gateway '%s' added and set as active\n", m.Name)

	return authenticateNewGateway(cmd, m.Name)
}

// authenticateNewGateway attempts to authenticate the just-registered
// gateway:
//   - a client secret is configured: client-credentials exchange, printing
//     the upstream-distinct "✓ Authenticated via client credentials" and
//     writing oidc_token.json with a real expires_at (the rosa-agent
//     CronJobs' Python token writer omits it; see metadata_marshal_test.go).
//   - no secret, OPENSHELL_NO_BROWSER set: register-only, print a hint, and
//     succeed (exit 0) — the interim behavior ROSAENG-68835 (the device-code
//     login spike) already specifies for this exact case, not a guess.
//   - no secret, browser available: fall back to the existing browser login
//     flow (loginAndReport), matching upstream's own default for a human
//     without a client secret.
//
// Resolves env fresh (not the Env from before WriteGateway wrote the gateway
// dir) — NewOSEnv snapshots whether the user config dir exists at call time,
// so reusing an Env obtained before the directory existed would see a nil
// UserFS even after the write.
func authenticateNewGateway(cmd *cobra.Command, name string) error {
	env, err := gatewayconfig.NewOSEnv()
	if err != nil {
		return err
	}
	target, err := gatewayconfig.Resolve(env, gatewayconfig.ResolveInput{Name: name})
	if err != nil {
		return err
	}

	secretProvider := clientSecretProvider(viper.GetString("client-secret-file"))
	if secretProvider == nil {
		if os.Getenv("OPENSHELL_NO_BROWSER") != "" {
			cmd.PrintErrln("Gateway registered. No client secret found — log in with " +
				"`openshellctl gateway login`, or export OPENSHELL_OIDC_CLIENT_SECRET.")
			return nil
		}
		return loginAndReport(cmd, target)
	}

	src, err := clientCredentialsSource(cmd, target, secretProvider)
	if err != nil {
		return err
	}
	tok, err := src.Token(cmd.Context())
	if err != nil {
		return err
	}
	cmd.Println("✓ Authenticated via client credentials")

	w, err := tokenWriterFor(target)
	if err != nil {
		return err
	}
	if w == nil {
		return nil
	}
	return auth.WriteBundle(w, tok)
}

// clientCredentialsSource returns the TokenSource for the client-credentials
// exchange: an injected cliDeps.TokenSource when present (tests), or a real
// auth.Resolve-built source otherwise. Production code never injects deps.
func clientCredentialsSource(cmd *cobra.Command, target *gatewayconfig.Target, secretProvider func(context.Context) (string, error)) (auth.TokenSource, error) {
	if deps, ok := depsFrom(cmd.Context()); ok && deps.TokenSource != nil {
		return deps.TokenSource, nil
	}

	in := auth.ResolveInput{
		ClientSecret:      secretProvider,
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
	return auth.Resolve(cmd.Context(), in, auth.NewSDKExchanger())
}
