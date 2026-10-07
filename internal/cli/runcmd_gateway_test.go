package cli

import (
	"context"
	"testing"
)

// runCmdWithGateway is runCmd (sandbox_cmd_test.go), but injects deps into
// the root command's context before executing, so commands that call
// withGatewayTarget/withGateway (or the resolveAuth/dialOrInjected wrappers
// in authwiring.go) use the injected gateway/target/token source instead of
// dialing for real. This is the seam this story adds.
func runCmdWithGateway(t *testing.T, deps cliDeps, args ...string) (string, error) {
	t.Helper()
	return runCmdCtx(t, withDeps(context.Background(), deps), args...)
}
