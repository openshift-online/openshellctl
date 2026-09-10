package cli

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/openshellctl/internal/version"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, OpenShell pin, and SDK version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := version.Get()
			out := cmd.OutOrStdout()
			cmd.SetOut(out)
			cmd.Printf("openshellctl %s\n", info.Version)
			cmd.Printf("  commit:        %s\n", info.Commit)
			cmd.Printf("  openshell-pin: %s\n", info.OpenShellPin)
			cmd.Printf("  sdk:           %s\n", info.SDKVersion)
			return nil
		},
	}
}
