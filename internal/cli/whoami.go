package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newWhoamiCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "whoami",
		Short: "Show the current authenticated identity",
		Long:  "Print the current user's identity as seen by the gateway. Use -v for full token details (same as `token show`).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			src, target, err := resolveTokenSource(cmd)
			if err != nil {
				return err
			}

			v, _ := cmd.Flags().GetCount("verbose")
			if v > 0 {
				tok, terr := src.Token(cmd.Context())
				if terr != nil {
					return terr
				}
				if err := writeToken(cmd.OutOrStdout(), tok, src.Describe(), "text"); err != nil {
					return err
				}
				reportCurrentUser(cmd, target, src, "text")
				return nil
			}

			gw, conn, err := dialGateway(target, src)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			user, err := gw.CurrentUser(cmd.Context())
			if err != nil {
				return err
			}

			cmd.Println("Current User")
			cmd.Println()
			cmd.Printf("  Subject: %s\n", user.Subject)
			if user.DisplayName != "" {
				cmd.Printf("  Name: %s\n", user.DisplayName)
			}
			cmd.Printf("  Provider: %s\n", user.IdentityProvider)
			cmd.Printf("  Roles: %s\n", strings.Join(user.Roles, ", "))
			cmd.Printf("  Scopes: %s\n", strings.Join(user.Scopes, ", "))
			return nil
		},
	}
	return c
}
