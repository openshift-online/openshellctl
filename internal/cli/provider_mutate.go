package cli

import (
	"fmt"
	"os"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/spf13/cobra"

	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/provider"
)

// profileWorkspace resolves --global-profile into the ProviderSpec.ProfileWorkspace
// value: a platform-scoped (global) profile is the empty string; otherwise
// it's the current workspace.
func profileWorkspace(globalProfile bool, workspace string) string {
	if globalProfile {
		return ""
	}
	return workspace
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
			if err := provider.ValidateCreateFlags(name, providerType, credentials, fromExisting, fromGcloudADC, runtimeCredentials); err != nil {
				return &UsageError{Err: err}
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
			spec := provider.BuildSpec(creds, config, expiry)
			spec.ProfileWorkspace = profileWorkspace(globalProfile, ws)

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
		globalProfile                                   bool
		globalProfileSet                                bool
	)
	c := &cobra.Command{
		Use:   "update NAME",
		Short: "Update an existing provider's credentials or config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			globalProfileSet = cmd.Flags().Changed("global-profile")
			if err := provider.ValidateUpdateFlags(fromExisting, fromGcloudADC, runtimeCredentials); err != nil {
				return &UsageError{Err: err}
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
				if globalProfileSet {
					updated.Spec.ProfileWorkspace = profileWorkspace(globalProfile, ws)
				}
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
	f.BoolVar(&globalProfile, "global-profile", false, "use a platform-scoped (global) provider profile")
	f.StringArrayVar(&expiresAt, "credential-expires-at", nil, "credential expiry (KEY=TIMESTAMP); epoch ms or RFC3339, 0 clears")
	return c
}
