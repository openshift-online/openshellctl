package sandbox

import (
	"context"
	"errors"
	"time"

	"github.com/openshift-online/openshellctl/pkg/gateway"
)

// replaceExisting implements --replace: if a sandbox named name already
// exists in workspace, delete it and wait for it to be gone before
// returning, so the subsequent create never races a deletion still in
// flight. A NotFoundError from the initial Get is not an error — there's
// simply nothing to replace. A Get or Delete RPC error (distinct from
// NotFound) propagates unwrapped, surfacing the real problem instead of the
// error-swallowing `openshell sandbox delete "$NAME" || true` CronJob
// pattern this feature replaces.
func replaceExisting(ctx context.Context, gw gateway.Gateway, workspace, name string, timeout time.Duration, clock Clock) error {
	return replaceExistingWithDeps(ctx, gw, workspace, name, timeout, waitGoneDeps{Clock: clock})
}

// replaceExistingWithDeps is replaceExisting with the full waitGoneDeps
// injection seam exposed, so tests can drive the wait-for-gone step's
// timeout/cancellation branches deterministically (see waitgone_test.go for
// why Tick/Clock injection exists).
func replaceExistingWithDeps(ctx context.Context, gw gateway.Gateway, workspace, name string, timeout time.Duration, d waitGoneDeps) error {
	_, err := gw.GetSandbox(ctx, workspace, name)
	var nf *gateway.NotFoundError
	if errors.As(err, &nf) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := gw.DeleteSandbox(ctx, workspace, name); err != nil {
		return err
	}
	return waitGone(ctx, gw, workspace, name, timeout, d)
}
