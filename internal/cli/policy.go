package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/openshellctl/pkg/policyyaml"
)

func newPolicyCommand() *cobra.Command {
	p := &cobra.Command{
		Use:   "policy",
		Short: "Work with sandbox policies",
	}
	p.AddCommand(newPolicyLintCommand())
	return p
}

func newPolicyLintCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "lint <file>",
		Short: "Check a sandbox policy for problems (client-side; the gateway is authoritative)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			policy, err := policyyaml.Parse(data)
			if err != nil {
				return err // parse errors (schema/type/unknown-field) surface here
			}
			findings := policyyaml.Lint(policy)
			if len(findings) == 0 {
				cmd.Printf("✓ %s: no problems found\n", args[0])
				return nil
			}
			for _, f := range findings {
				cmd.PrintErrf("  ✗ %s\n", f.Error())
			}
			return fmt.Errorf("%s: %d problem(s) found", args[0], len(findings))
		},
	}
}
