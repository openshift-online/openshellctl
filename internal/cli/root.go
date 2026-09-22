// Package cli implements the openshellctl cobra/viper command surface. Command
// files hold only flag wiring and thin adapters; all logic lives in pkg/*.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// envKeyReplacer maps viper config keys to OPENSHELL_ env var names: "-" and
// "." both become "_" (e.g. gateway-endpoint -> OPENSHELL_GATEWAY_ENDPOINT,
// oidc.client_secret -> OPENSHELL_OIDC_CLIENT_SECRET), per §5.9.
func envKeyReplacer() *strings.Replacer {
	return strings.NewReplacer("-", "_", ".", "_")
}

// persistentFlags holds the values bound to the root persistent flags. Command
// implementations read these (via the *cobra.Command tree) rather than package
// globals; the struct is attached to the root command's context in later commits.
type persistentFlags struct {
	gateway         string
	gatewayEndpoint string
	gatewayInsecure bool
	workspace       string
	verbosity       int
	configFile      string
	noColor         bool

	// auth
	token            string
	clientSecretFile string
	oidcIssuer       string
	oidcClientID     string
	oidcAudience     string
	oidcScopes       string
	tokenLeeway      string
	writeToken       bool
}

// NewRootCommand builds the full command tree. It is exported so tests (parity,
// execution) can construct a fresh tree without global state.
func NewRootCommand() *cobra.Command {
	pf := &persistentFlags{}

	root := &cobra.Command{
		Use:           "openshellctl",
		Short:         "Lightweight Go client for OpenShell",
		SilenceUsage:  true,
		SilenceErrors: true,
		// DisableFlagsInUseLine keeps the usage line clean (spec §5.9).
		DisableFlagsInUseLine: true,
	}

	// Flag parse errors map to exit code 2 (usage).
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &UsageError{Err: err}
	})

	registerPersistentFlags(root, pf)
	bindViper(root)

	root.AddCommand(
		newSandboxCommand(),
		newLogsCommand(),
		newTokenCommand(),
		newPolicyCommand(),
		newVersionCommand(),
		newWhoamiCommand(),
		newLoginCommand(),
	)

	return root
}

func registerPersistentFlags(root *cobra.Command, pf *persistentFlags) {
	f := root.PersistentFlags()
	f.StringVarP(&pf.gateway, "gateway", "g", "", "gateway name (env OPENSHELL_GATEWAY)")
	f.StringVar(&pf.gatewayEndpoint, "gateway-endpoint", "", "gateway endpoint URL (env OPENSHELL_GATEWAY_ENDPOINT)")
	f.BoolVar(&pf.gatewayInsecure, "gateway-insecure", false, "skip TLS verification (env OPENSHELL_GATEWAY_INSECURE)")
	f.StringVar(&pf.workspace, "workspace", "default", "workspace (env OPENSHELL_WORKSPACE)")
	f.CountVarP(&pf.verbosity, "verbose", "v", "increase verbosity")
	f.StringVar(&pf.configFile, "config", "", "config file (default $XDG_CONFIG_HOME/openshellctl/config.yaml)")
	f.BoolVar(&pf.noColor, "no-color", false, "disable colored output")

	f.StringVar(&pf.token, "token", "", "bearer token (env OPENSHELL_TOKEN)")
	f.StringVar(&pf.clientSecretFile, "client-secret-file", "", "file containing the OIDC client secret")
	f.StringVar(&pf.oidcIssuer, "oidc-issuer", "", "OIDC issuer URL override")
	f.StringVar(&pf.oidcClientID, "oidc-client-id", "", "OIDC client id override")
	f.StringVar(&pf.oidcAudience, "oidc-audience", "", "OIDC audience override")
	f.StringVar(&pf.oidcScopes, "oidc-scopes", "", "OIDC scopes override (space-separated)")
	f.StringVar(&pf.tokenLeeway, "token-leeway", "30s", "token expiry leeway")
	f.BoolVar(&pf.writeToken, "write-token", false, "write refreshed tokens back to disk (Rust CLI schema)")
}

// bindViper wires the OPENSHELL_* environment prefix and flag binding, per §5.9.
func bindViper(root *cobra.Command) {
	viper.SetEnvPrefix("OPENSHELL")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(envKeyReplacer())
	// Persistent flags are bound to viper so env vars back them.
	_ = viper.BindPFlags(root.PersistentFlags())
}

// Execute builds and runs the root command, returning a process exit code.
// It prints "Error: <msg>" to stderr for failures (root has SilenceErrors set).
// SIGINT and SIGTERM cancel the command context so non-interactive operations
// (watch, upload, gRPC calls) clean up promptly. During raw-mode SSH sessions,
// Ctrl-C (0x03) flows through stdin to the remote process — the terminal
// driver does not convert it to SIGINT while in raw mode.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	root := NewRootCommand()
	root.SetContext(ctx)
	if err := root.Execute(); err != nil {
		// A remote (exec/connect/create-attach) non-zero exit is propagated as the
		// process status without an "Error:" message — the remote command already
		// wrote its own output.
		var remote *RemoteExitError
		if !errors.As(err, &remote) && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		}
		code := exitCodeFor(err)
		if code == ExitAuth {
			msg := err.Error()
			if !strings.Contains(msg, "openshellctl token refresh") && !strings.Contains(msg, "openshellctl login") {
				fmt.Fprintf(os.Stderr, "Hint: try `openshellctl token refresh` to obtain a new token.\n")
			}
		}
		return code
	}
	return ExitOK
}
