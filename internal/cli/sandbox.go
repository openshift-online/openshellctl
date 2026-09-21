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

// notImplementedLeaf returns a leaf command whose RunE reports NotImplementedError.
func notImplementedLeaf(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(_ *cobra.Command, _ []string) error {
			return &NotImplementedError{Command: use}
		},
	}
}
