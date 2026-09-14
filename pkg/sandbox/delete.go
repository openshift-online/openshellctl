package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
)

// DeleteRequest is the input to Delete.
type DeleteRequest struct {
	Workspace   string
	Names       []string
	All         bool
	Wait        bool
	WaitTimeout time.Duration
}

// DeleteOutcome reports the result for one sandbox.
type DeleteOutcome struct {
	Name    string
	Deleted bool
}

// ErrNothingToDelete is returned by --all when no sandboxes exist. The CLI
// renders the verbatim upstream message "No sandboxes to delete." for it.
var ErrNothingToDelete = errors.New("no sandboxes to delete")

// NothingToDeleteMessage is the verbatim upstream user-facing string.
const NothingToDeleteMessage = "No sandboxes to delete."

// ErrDeleteTimeout is returned when --wait times out.
type ErrDeleteTimeout struct {
	Name  string
	After time.Duration
}

func (e *ErrDeleteTimeout) Error() string {
	return fmt.Sprintf("timed out after %s waiting for sandbox %q to be deleted", e.After, e.Name)
}

// Delete deletes sandboxes (run.rs:2350-2414). With All it lists the workspace
// (limit 1000); empty → ErrNothingToDelete. Per name it calls DeleteSandbox;
// on deleted==true it clears last_sandbox and reports. The first RPC error
// aborts. Wait (extension) polls GetSandbox every 500ms until NotFound or
// timeout. cfgw may be nil (no last_sandbox clearing).
func Delete(ctx context.Context, gw gateway.Gateway, cfgw gatewayconfig.Writer, gatewayName string, r DeleteRequest, report func(DeleteOutcome)) error {
	names := r.Names
	if r.All {
		list, err := gw.ListSandboxes(ctx, r.Workspace, types.ListOptions{Limit: 1000})
		if err != nil {
			return err
		}
		if len(list) == 0 {
			return ErrNothingToDelete
		}
		names = names[:0]
		for _, s := range list {
			names = append(names, s.Name)
		}
	}

	for _, name := range names {
		deleted, err := gw.DeleteSandbox(ctx, r.Workspace, name)
		if err != nil {
			return err
		}
		if deleted && cfgw != nil && gatewayName != "" {
			_ = gatewayconfig.ClearLastSandboxIfMatches(cfgw, gatewayName, r.Workspace, name)
		}
		if report != nil {
			report(DeleteOutcome{Name: name, Deleted: deleted})
		}
		if r.Wait && deleted {
			if err := waitForDeletion(ctx, gw, r.Workspace, name, r.WaitTimeout); err != nil {
				return err
			}
		}
	}
	return nil
}

// waitForDeletion polls GetSandbox every 500ms until NotFound or timeout.
func waitForDeletion(ctx context.Context, gw gateway.Gateway, workspace, name string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := gw.GetSandbox(ctx, workspace, name)
		var nf *gateway.NotFoundError
		if errors.As(err, &nf) {
			return nil
		}
		if time.Now().After(deadline) {
			return &ErrDeleteTimeout{Name: name, After: timeout}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
