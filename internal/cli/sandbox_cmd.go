package cli

import (
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/openshift-online/openshellctl/pkg/gateway"
)

// withGateway resolves the token source + target, dials the gateway, and invokes
// fn with a ready Gateway. The connection is closed on return. Errors from
// resolution/dial are returned (mapped to exit codes by Execute).
func withGateway(cmd *cobra.Command, fn func(gw gateway.Gateway) error) error {
	src, target, err := resolveTokenSource(cmd)
	if err != nil {
		return err
	}
	gw, conn, err := dialGateway(target, src)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return fn(gw)
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
