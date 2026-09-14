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
		notImplementedLeaf("connect", "Connect to a sandbox"),
		notImplementedLeaf("upload", "Upload files to a sandbox"),
		notImplementedLeaf("download", "Download files from a sandbox"),
		notImplementedLeaf("ssh-config", "Print an ssh config block for a sandbox"),
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
		notImplementedLeaf("list", "List providers attached to a sandbox"),
		notImplementedLeaf("attach", "Attach a provider to a sandbox"),
		notImplementedLeaf("detach", "Detach a provider from a sandbox"),
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
