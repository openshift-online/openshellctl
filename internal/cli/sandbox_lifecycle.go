package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/sandbox"
)

func newSandboxStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop [NAME]",
		Short: "Stop a sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGateway(cmd, func(gw gateway.Gateway) error {
				if _, err := sandbox.Stop(cmd.Context(), gw, workspace(), args[0], lifecycleTimeout()); err != nil {
					return err
				}
				cmd.Printf("✓ Stopped sandbox %s\n", args[0])
				return nil
			})
		},
	}
}

func newSandboxStartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start [NAME]",
		Short: "Start a sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGateway(cmd, func(gw gateway.Gateway) error {
				if _, err := sandbox.Start(cmd.Context(), gw, workspace(), args[0], lifecycleTimeout()); err != nil {
					return err
				}
				cmd.Printf("✓ Started sandbox %s\n", args[0])
				return nil
			})
		},
	}
}

func newSandboxDeleteCommand() *cobra.Command {
	var (
		all         bool
		wait        bool
		waitTimeout time.Duration
	)
	c := &cobra.Command{
		Use:   "delete NAME...",
		Short: "Delete sandboxes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !all && len(args) == 0 {
				return &UsageError{Err: fmt.Errorf("a sandbox name is required unless --all is given")}
			}
			if all && len(args) > 0 {
				return &UsageError{Err: fmt.Errorf("--all cannot be combined with names")}
			}
			return withGateway(cmd, func(gw gateway.Gateway) error {
				req := sandbox.DeleteRequest{
					Workspace:   workspace(),
					Names:       args,
					All:         all,
					Wait:        wait,
					WaitTimeout: waitTimeout,
				}
				err := sandbox.Delete(cmd.Context(), gw, nil, "", req, func(o sandbox.DeleteOutcome) {
					if o.Deleted {
						cmd.Printf("✓ Deleted sandbox %s\n", o.Name)
					} else {
						cmd.Printf("! Sandbox %s not found\n", o.Name)
					}
				})
				if errors.Is(err, sandbox.ErrNothingToDelete) {
					cmd.Println(sandbox.NothingToDeleteMessage)
					return nil
				}
				return err
			})
		},
	}
	f := c.Flags()
	f.BoolVar(&all, "all", false, "delete all sandboxes in the workspace")
	f.BoolVar(&wait, "wait", false, "wait for deletion to complete")
	f.DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, "wait timeout")
	return c
}

// lifecycleTimeout reads OPENSHELL_LIFECYCLE_TIMEOUT (seconds), default 300s.
func lifecycleTimeout() time.Duration {
	return envSecondsDefault("OPENSHELL_LIFECYCLE_TIMEOUT", 300*time.Second)
}
