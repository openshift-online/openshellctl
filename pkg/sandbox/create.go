package sandbox

import (
	"context"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/openshift-online/openshellctl/pkg/gateway"
)

// CreateDeps are the dependencies for Create. Transfer/attach steps (uploads,
// forward, interactive connect) are gated on future fields being set; this
// create-without-transfer path (spec §8.4) covers provider resolution, spec
// build, the SDK/raw CreateSandbox split, and last_sandbox persistence.
type CreateDeps struct {
	GW  gateway.Gateway
	Env func(string) string
}

// CreateResult is the outcome of Create.
type CreateResult struct {
	Sandbox *types.Sandbox
}

// Create builds the spec and creates the sandbox. It resolves providers against
// the gateway (existing names kept; unknown recognised types are reported via
// ErrAutoProviderUnsupported unless --no-auto-providers skips them), then calls
// CreateSandbox. Bare --gpu is not yet routed through the raw path here (that
// lands with the attach/transfer work); an explicit count uses the SDK spec.
//
// This is the create-without-transfer path: it does not watch-to-ready, upload,
// forward, or attach — the CLI decides output/attach policy on the returned
// sandbox (§8.4 scope).
func Create(ctx context.Context, d CreateDeps, r *CreateRequest, ttyResolved bool) (*CreateResult, error) {
	if err := resolveRequestProviders(ctx, d.GW, r); err != nil {
		return nil, err
	}

	spec := ToSDKSpec(r, ttyResolved)
	sb, err := d.GW.CreateSandbox(ctx, r.Workspace, r.Name, spec, r.Labels)
	if err != nil {
		return nil, err
	}
	return &CreateResult{Sandbox: sb}, nil
}

// resolveRequestProviders resolves r.Providers against the gateway's providers.
// Recognised-but-missing types are surfaced as ErrAutoProviderUnsupported
// (openshellctl cannot auto-create) unless AutoProviders is explicitly false, in
// which case they are dropped.
func resolveRequestProviders(ctx context.Context, gw gateway.Gateway, r *CreateRequest) error {
	if len(r.Providers) == 0 {
		return nil
	}
	known, err := listAllProviders(ctx, gw, r.Workspace)
	if err != nil {
		return err
	}
	res, err := ResolveProviders(known, r.Providers, nil)
	if err != nil {
		return err
	}
	if len(res.MissingTypes) > 0 {
		if r.AutoProviders != nil && !*r.AutoProviders {
			// Skip missing types (caller prints the skip message).
			r.Providers = res.Names
			return nil
		}
		return &ErrAutoProviderUnsupported{Type: res.MissingTypes[0]}
	}
	r.Providers = res.Names
	return nil
}

// listAllProviders pages ListProviders by 100 until a short page (run.rs:2561).
func listAllProviders(ctx context.Context, gw gateway.Gateway, workspace string) ([]*types.Provider, error) {
	var all []*types.Provider
	offset := 0
	for {
		page, err := gw.ListProviders(ctx, workspace, types.ListOptions{Limit: 100, Offset: offset})
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < 100 {
			break
		}
		offset += len(page)
	}
	return all, nil
}
