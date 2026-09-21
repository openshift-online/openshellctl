package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
	"github.com/openshift-online/openshellctl/pkg/transfer"
)

func newSandboxSSHConfigCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "ssh-config [NAME]",
		Short: "Print an ssh config block for a sandbox",
		Long:  "Print an SSH configuration block for use with the upstream openshell binary.\nThis is informational only — it requires the upstream `openshell` binary to use.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, target, err := resolveTokenSource(cmd)
			if err != nil {
				return err
			}
			_ = src

			ws := workspace()
			sbName, err := resolveSandboxName(args, target, ws)
			if err != nil {
				return err
			}

			block := SSHConfigBlock(sbName, ws, target)
			_, err = fmt.Fprint(cmd.OutOrStdout(), block)
			return err
		},
	}
	return c
}

// SSHConfigBlock builds the ssh_config(5) block from A.12 (ssh.rs:1503-1517).
// shellEscape is applied to the ProxyCommand arguments. The gateway name comes
// from the resolved target; if unavailable, falls back to "default". Pure.
func SSHConfigBlock(name, workspace string, target *gatewayconfig.Target) string {
	gwName := "default"
	if target != nil && target.Name != "" {
		gwName = target.Name
	}

	exe, err := os.Executable()
	if err != nil {
		exe = "openshell"
	}

	return fmt.Sprintf(`Host openshell-%s.%s
  User sandbox
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  GlobalKnownHostsFile /dev/null
  LogLevel ERROR
  ServerAliveInterval 15
  ServerAliveCountMax 3
  ProxyCommand %s ssh-proxy --gateway-name %s --name %s --workspace %s
`,
		name, workspace,
		transfer.ShellEscape(exe),
		transfer.ShellEscape(gwName),
		transfer.ShellEscape(name),
		transfer.ShellEscape(workspace),
	)
}
