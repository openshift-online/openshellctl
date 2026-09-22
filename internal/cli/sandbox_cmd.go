package cli

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
)

// withGateway resolves the token source + target, dials the gateway, and invokes
// fn with a ready Gateway. The connection is closed on return. Errors from
// resolution/dial are returned (mapped to exit codes by Execute).
func withGateway(cmd *cobra.Command, fn func(gw gateway.Gateway) error) error {
	return withGatewayTarget(cmd, func(gw gateway.Gateway, _ *gatewayconfig.Target) error {
		return fn(gw)
	})
}

// withGatewayTarget is withGateway but also passes the resolved target, so
// callers can read/write last_sandbox and the gateway name.
func withGatewayTarget(cmd *cobra.Command, fn func(gw gateway.Gateway, target *gatewayconfig.Target) error) error {
	src, target, err := resolveTokenSource(cmd)
	if err != nil {
		return err
	}
	gw, conn, err := dialGateway(target, src)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return fn(gw, target)
}

// resolveSandboxName returns the explicit name argument, or falls back to the
// gateway's last_sandbox for the workspace. It errors (usage) when neither is
// available (A.3: "defaults to last-used sandbox").
func resolveSandboxName(args []string, target *gatewayconfig.Target, ws string) (string, error) {
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}
	if target != nil && target.Resolved != nil {
		if name, ok := gatewayconfig.LoadLastSandbox(target.Resolved, ws); ok {
			return name, nil
		}
	}
	return "", &UsageError{Err: fmt.Errorf("a sandbox name is required (no last-used sandbox found)")}
}

// resolveNameFromFileOrArgs resolves a sandbox name from a -f manifest file or
// from positional/--name args. The two sources are mutually exclusive: if both
// are provided, a usage error is returned. When file is empty, resolution falls
// through to resolveSandboxName(args, ...).
func resolveNameFromFileOrArgs(cmd *cobra.Command, file string, args []string, target *gatewayconfig.Target, ws string) (string, error) {
	if file != "" {
		if len(args) > 0 && args[0] != "" {
			return "", &UsageError{Err: fmt.Errorf("cannot combine -f/--file with a positional name or --name")}
		}
		names, err := namesFromManifest(cmd, file)
		if err != nil {
			return "", err
		}
		return names[0], nil
	}
	return resolveSandboxName(args, target, ws)
}

// saveLastSandbox best-effort records the workspace/name as the gateway's
// last_sandbox. Failures are ignored (matching the CLI's `let _ = ...`).
func saveLastSandbox(target *gatewayconfig.Target, ws, name string) {
	if target == nil || target.Name == "" {
		return
	}
	w, err := gatewayconfig.NewOSWriter()
	if err != nil {
		return
	}
	_ = gatewayconfig.SaveLastSandbox(w, target.Name, ws, name)
}

// workspace returns the effective workspace (flag/env via viper, default "default").
func workspace() string {
	ws := viper.GetString("workspace")
	if ws == "" {
		return "default"
	}
	return ws
}

// envSecondsDefault reads an integer-seconds env var, falling back to def.
func envSecondsDefault(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return def
}
