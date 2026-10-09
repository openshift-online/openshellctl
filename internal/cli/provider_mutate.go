package cli

import (
	"fmt"
	"os"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/spf13/cobra"

	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/provider"
)

// unsupportedCredentialSourceFlags checks the three provider credential
// sources openshellctl doesn't implement yet (reading local state, gcloud
// ADC, or gateway-resolved runtime credentials), returning a usage error
// naming whichever was passed. nil when none were.
func unsupportedCredentialSourceFlags(fromExisting, fromGcloudADC, runtimeCredentials bool) error {
	switch {
	case fromExisting:
		return &UsageError{Err: fmt.Errorf("--from-existing is not yet supported in openshellctl; run `openshell provider create --from-existing ...` directly")}
	case fromGcloudADC:
		return &UsageError{Err: fmt.Errorf("--from-gcloud-adc is not yet supported in openshellctl; run `openshell provider create --from-gcloud-adc ...` directly")}
	case runtimeCredentials:
		return &UsageError{Err: fmt.Errorf("--runtime-credentials is not yet supported in openshellctl; run `openshell provider create --runtime-credentials ...` directly")}
	default:
		return nil
	}
}

func newProviderCreateCommand() *cobra.Command {
	var (
		name, providerType                              string
		credentials, configPairs, expiresAt             []string
		fromExisting, fromGcloudADC, runtimeCredentials bool
		globalProfile                                   bool
	)
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a provider config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return &UsageError{Err: fmt.Errorf("--name is required")}
			}
			if providerType == "" {
				return &UsageError{Err: fmt.Errorf("--type is required")}
			}
			if err := unsupportedCredentialSourceFlags(fromExisting, fromGcloudADC, runtimeCredentials); err != nil {
				return err
			}
			if len(credentials) == 0 {
				return &UsageError{Err: fmt.Errorf("at least one --credential is required (or use --from-existing/--from-gcloud-adc/--runtime-credentials, not yet supported in openshellctl)")}
			}

			creds, err := provider.ParseCredentialPairs(credentials, os.Getenv)
			if err != nil {
				return &UsageError{Err: err}
			}
			config, err := provider.ParseConfigPairs(configPairs)
			if err != nil {
				return &UsageError{Err: err}
			}
			expiry, err := provider.ParseCredentialExpiresAt(expiresAt)
			if err != nil {
				return &UsageError{Err: err}
			}

			ws := workspace()
			profileWorkspace := ws
			if globalProfile {
				profileWorkspace = ""
			}
			spec := provider.BuildSpec(creds, config, expiry)
			spec.ProfileWorkspace = profileWorkspace

			return withGateway(cmd, func(gw gateway.Gateway) error {
				p := &types.Provider{Name: name, Type: providerType, Workspace: ws, Spec: spec}
				created, err := gw.CreateProvider(cmd.Context(), ws, p)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✓ Created provider %s\n", created.Name)
				return nil
			})
		},
	}
	f := c.Flags()
	f.StringVar(&name, "name", "", "provider name")
	f.StringVar(&providerType, "type", "", "provider type")
	f.BoolVar(&fromExisting, "from-existing", false, "load provider credentials/config from existing local state")
	f.StringArrayVar(&credentials, "credential", nil, "provider credential pair (KEY=VALUE) or env lookup key (KEY)")
	f.BoolVar(&fromGcloudADC, "from-gcloud-adc", false, "configure credentials from gcloud Application Default Credentials")
	f.BoolVar(&runtimeCredentials, "runtime-credentials", false, "create a provider whose credentials are resolved at runtime")
	f.StringArrayVar(&configPairs, "config", nil, "provider config key/value pair")
	f.BoolVar(&globalProfile, "global-profile", false, "use a platform-scoped (global) provider profile")
	f.StringArrayVar(&expiresAt, "credential-expires-at", nil, "credential expiry (KEY=TIMESTAMP); epoch ms or RFC3339, 0 clears")
	return c
}

func newProviderUpdateCommand() *cobra.Command {
	var (
		credentials, configPairs, expiresAt             []string
		fromExisting, fromGcloudADC, runtimeCredentials bool
	)
	c := &cobra.Command{
		Use:   "update NAME",
		Short: "Update an existing provider's credentials or config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := unsupportedCredentialSourceFlags(fromExisting, fromGcloudADC, runtimeCredentials); err != nil {
				return err
			}

			creds, err := provider.ParseCredentialPairs(credentials, os.Getenv)
			if err != nil {
				return &UsageError{Err: err}
			}
			config, err := provider.ParseConfigPairs(configPairs)
			if err != nil {
				return &UsageError{Err: err}
			}
			expiry, err := provider.ParseCredentialExpiresAt(expiresAt)
			if err != nil {
				return &UsageError{Err: err}
			}

			return withGateway(cmd, func(gw gateway.Gateway) error {
				ws := workspace()
				existing, err := gw.GetProvider(cmd.Context(), ws, args[0])
				if err != nil {
					return err
				}
				overlay := types.ProviderSpec{Credentials: creds, Config: config, CredentialExpiresAt: expiry}
				updated := *existing
				updated.Spec = provider.MergeSpec(existing.Spec, overlay)
				result, err := gw.UpdateProvider(cmd.Context(), ws, &updated)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✓ Updated provider %s\n", result.Name)
				return nil
			})
		},
	}
	f := c.Flags()
	f.BoolVar(&fromExisting, "from-existing", false, "load provider credentials/config from existing local state")
	f.StringArrayVar(&credentials, "credential", nil, "provider credential pair (KEY=VALUE) or env lookup key (KEY)")
	f.BoolVar(&fromGcloudADC, "from-gcloud-adc", false, "configure credentials from gcloud Application Default Credentials")
	f.BoolVar(&runtimeCredentials, "runtime-credentials", false, "resolve credentials at runtime")
	f.StringArrayVar(&configPairs, "config", nil, "provider config key/value pair")
	f.StringArrayVar(&expiresAt, "credential-expires-at", nil, "credential expiry (KEY=TIMESTAMP); epoch ms or RFC3339, 0 clears")
	return c
}
