package cli

import "github.com/spf13/cobra"

// newSandboxCommand builds the `sandbox` (alias `sb`) parent and its
// subcommands. In this scaffold the leaf commands are stubs returning
// NotImplementedError; they are fleshed out in later commits. The full tree
// exists now so the parity test can enumerate every subcommand.
func newSandboxCommand() *cobra.Command {
	sb := &cobra.Command{
		Use:     "sandbox",
		Aliases: []string{"sb"},
		Short:   "Manage sandboxes",
	}

	// TODO: add `sandbox validate` subcommand — client-side validation of manifests
	// against OpenShell restrictions (name length ≤19, image format, resource
	// quantities, label constraints) before sending to the gateway.

	// TODO: add `sandbox convert -f <file>` subcommand — read a YAML sandbox
	// manifest and print the equivalent `openshell sandbox create` CLI command.
	// Useful for users migrating from declarative manifests to one-liners and
	// for debugging what flags a manifest resolves to.
	sb.AddCommand(
		newSandboxCreateCommand(),
		newSandboxGetCommand(),
		newSandboxListCommand(),
		newSandboxDeleteCommand(),
		newSandboxStopCommand(),
		newSandboxStartCommand(),
		newSandboxExecCommand(),
		newSandboxConnectCommand(),
		newSandboxUploadCommand(),
		newSandboxDownloadCommand(),
		newSandboxSSHConfigCommand(),
		newProviderCommand(),
	)

	return sb
}

// newProviderCommand builds `sandbox provider` and its list/attach/detach leaves.
func newProviderCommand() *cobra.Command {
	p := &cobra.Command{
		Use:   "provider",
		Short: "Manage sandbox providers",
	}
	p.AddCommand(
		newProviderListCommand(),
		newProviderAttachCommand(),
		newProviderDetachCommand(),
	)
	return p
}
