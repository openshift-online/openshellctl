package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
	"github.com/openshift-online/openshellctl/pkg/output"
)

func newProviderListCommand() *cobra.Command {
	var file string
	c := &cobra.Command{
		Use:   "list [NAME]",
		Short: "List providers attached to a sandbox",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGatewayTarget(cmd, func(gw gateway.Gateway, target *gatewayconfig.Target) error {
				ws := workspace()
				sbName, err := resolveNameFromFileOrArgs(cmd, file, args, target, ws)
				if err != nil {
					return err
				}
				providers, err := gw.ListSandboxProviders(cmd.Context(), ws, sbName)
				if err != nil {
					return err
				}
				return output.RenderProviderList(cmd.OutOrStdout(), sbName, providers)
			})
		},
	}
	c.Flags().StringVarP(&file, "file", "f", "", "manifest file to read sandbox name from (- for stdin)")
	return c
}

func newProviderAttachCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "attach NAME PROVIDER",
		Short: "Attach a provider to a sandbox",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sbName := args[0]
			provider := args[1]
			return withGatewayTarget(cmd, func(gw gateway.Gateway, target *gatewayconfig.Target) error {
				ws := workspace()
				sb, err := gw.GetSandbox(cmd.Context(), ws, sbName)
				if err != nil {
					return err
				}
				_, noop, err := gw.AttachProvider(cmd.Context(), ws, sbName, provider, sb.ResourceVersion)
				if err != nil {
					var conflict *gateway.ConflictError
					if errors.As(err, &conflict) {
						return fmt.Errorf("failed to attach provider: sandbox was modified by another operation, please retry the command")
					}
					return err
				}
				if noop {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Provider %s is already attached to sandbox %s.\n", provider, sbName)
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✓ Attached provider %s to sandbox %s\n", provider, sbName)
				}
				return nil
			})
		},
	}
}

func newProviderDetachCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "detach NAME PROVIDER",
		Short: "Detach a provider from a sandbox",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sbName := args[0]
			provider := args[1]
			return withGatewayTarget(cmd, func(gw gateway.Gateway, target *gatewayconfig.Target) error {
				ws := workspace()
				sb, err := gw.GetSandbox(cmd.Context(), ws, sbName)
				if err != nil {
					return err
				}
				_, noop, err := gw.DetachProvider(cmd.Context(), ws, sbName, provider, sb.ResourceVersion)
				if err != nil {
					var conflict *gateway.ConflictError
					if errors.As(err, &conflict) {
						return fmt.Errorf("failed to detach provider: sandbox was modified by another operation, please retry the command")
					}
					return err
				}
				if noop {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Provider %s was not attached to sandbox %s.\n", provider, sbName)
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✓ Detached provider %s from sandbox %s\n", provider, sbName)
				}
				return nil
			})
		},
	}
}
