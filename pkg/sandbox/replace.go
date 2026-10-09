package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
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
// pattern this feature replaces. stderr receives a "Replacing existing
// sandbox" progress line, but only when there is actually something to
// replace (nil is safe — no write is attempted).
func replaceExisting(ctx context.Context, gw gateway.Gateway, workspace, name string, timeout time.Duration, clock Clock, stderr io.Writer) error {
	return replaceExistingWithDeps(ctx, gw, workspace, name, timeout, waitGoneDeps{Clock: clock}, stderr)
}

// replaceExistingWithDeps is replaceExisting with the full waitGoneDeps
// injection seam exposed, so tests can drive the wait-for-gone step's
// timeout/cancellation branches deterministically (see waitgone_test.go for
// why Tick/Clock injection exists).
func replaceExistingWithDeps(ctx context.Context, gw gateway.Gateway, workspace, name string, timeout time.Duration, d waitGoneDeps, stderr io.Writer) error {
	_, err := gw.GetSandbox(ctx, workspace, name)
	var nf *gateway.NotFoundError
	if errors.As(err, &nf) {
		return nil
	}
	if err != nil {
		return err
	}
	if stderr != nil {
		_, _ = fmt.Fprintf(stderr, "Replacing existing sandbox %s...\n", name)
	}
	if _, err := gw.DeleteSandbox(ctx, workspace, name); err != nil {
		return err
	}
	return waitGone(ctx, gw, workspace, name, timeout, d)
}
