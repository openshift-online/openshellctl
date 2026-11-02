package cli

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestEnvKeyReplacer(t *testing.T) {
	r := envKeyReplacer()
	tests := map[string]string{
		"gateway-endpoint":   "gateway_endpoint",
		"oidc.client_secret": "oidc_client_secret",
		"gateway":            "gateway",
		"oidc-scopes":        "oidc_scopes",
	}
	for in, want := range tests {
		if got := r.Replace(in); got != want {
			t.Errorf("replace(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestViperEnvPrefixBinding(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	root := NewRootCommand()
	_ = root // building the root wires viper (SetEnvPrefix/AutomaticEnv/replacer)

	t.Setenv("OPENSHELL_GATEWAY", "rosa")
	t.Setenv("OPENSHELL_GATEWAY_ENDPOINT", "https://gw.example.com")

	if got := viper.GetString("gateway"); got != "rosa" {
		t.Errorf("viper gateway = %q, want rosa", got)
	}
	if got := viper.GetString("gateway-endpoint"); got != "https://gw.example.com" {
		t.Errorf("viper gateway-endpoint = %q, want the endpoint", got)
	}
}

func TestContextCanceledExitCode(t *testing.T) {
	if got := exitCodeFor(context.Canceled); got != ExitError {
		t.Errorf("context.Canceled exit code = %d, want %d", got, ExitError)
	}
}

func TestFlagParseErrorIsUsage(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := NewRootCommand()
	root.SetArgs([]string{"--nonexistent-flag"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
	if got := exitCodeFor(err); got != ExitUsage {
		t.Errorf("unknown flag exit code = %d, want %d (usage)", got, ExitUsage)
	}
}

// TestInsecureWarning confirms the pure function returns "" when insecure is
// false and a non-empty string containing the expected substrings when true.
func TestInsecureWarning(t *testing.T) {
	tests := []struct {
		name           string
		insecure       bool
		wantEmpty      bool
		wantSubstrings []string
	}{
		{
			name:      "false returns empty string",
			insecure:  false,
			wantEmpty: true,
		},
		{
			name:     "true returns warning containing required phrases",
			insecure: true,
			wantSubstrings: []string{
				"verification is disabled",
				"OIDC issuer",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := insecureWarning(tc.insecure)
			if tc.wantEmpty {
				if got != "" {
					t.Errorf("insecureWarning(false) = %q, want empty string", got)
				}
				return
			}
			for _, sub := range tc.wantSubstrings {
				if !strings.Contains(got, sub) {
					t.Errorf("insecureWarning(true) = %q, missing %q", got, sub)
				}
			}
		})
	}
}

// TestGatewayInsecureFlag_OverridesDefaultTransport confirms --gateway-insecure
// swaps http.DefaultTransport to one that skips TLS verification. This is the
// only lever available to make the OpenShell SDK's internal OIDC
// discovery/token HTTP client (an unexported package-level *http.Client with
// no Transport override, hence http.DefaultTransport) respect the flag — the
// SDK exposes no option to inject a custom client or skip verification.
// Stderr is intentionally uncaptured here; the warning output path is covered
// by TestGatewayInsecureFlag_PrintsWarning.
func TestGatewayInsecureFlag_OverridesDefaultTransport(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })

	root := NewRootCommand()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root.SetArgs([]string{"--gateway-insecure", "version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatalf("DefaultTransport = %T, want *http.Transport", http.DefaultTransport)
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be true after --gateway-insecure")
	}
	// The replacement must be a clone of the real default transport, not a
	// bare &http.Transport{} — a bare one silently drops proxy support
	// (HTTPS_PROXY/NO_PROXY), dial/handshake timeouts, keepalives, and
	// HTTP/2, which would regress every HTTP call in the process the moment
	// --gateway-insecure is set, not just the TLS verification it's meant to
	// relax.
	if tr.Proxy == nil {
		t.Error("expected the replacement transport to keep Proxy (http.ProxyFromEnvironment), got nil")
	}
	if tr.DialContext == nil {
		t.Error("expected the replacement transport to keep DialContext (dial timeouts), got nil")
	}
}

// TestGatewayInsecureFlag_PrintsWarning confirms that PersistentPreRunE prints
// the insecure warning to stderr (via cmd.ErrOrStderr()) when --gateway-insecure
// is active, and that the env-var form triggers the same warning.
func TestGatewayInsecureFlag_PrintsWarning(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		envKey  string
		envVal  string
		wantSub string
	}{
		{
			name:    "flag form prints warning",
			args:    []string{"--gateway-insecure", "version"},
			wantSub: "verification is disabled",
		},
		{
			name:    "env var form prints warning",
			args:    []string{"version"},
			envKey:  "OPENSHELL_GATEWAY_INSECURE",
			envVal:  "1",
			wantSub: "verification is disabled",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			original := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = original })

			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			if tc.envKey != "" {
				t.Setenv(tc.envKey, tc.envVal)
			}

			var buf strings.Builder
			root := NewRootCommand()
			root.SetArgs(tc.args)
			root.SetErr(&buf)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}

			if got := buf.String(); !strings.Contains(got, tc.wantSub) {
				t.Errorf("stderr = %q, want substring %q", got, tc.wantSub)
			}
		})
	}
}

// (no --gateway-insecure) never touches the global transport.
func TestGatewayInsecureFlag_NotSetLeavesTransportAlone(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })

	root := NewRootCommand()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var buf strings.Builder
	root.SetArgs([]string{"version"})
	root.SetErr(&buf)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if http.DefaultTransport != original {
		t.Error("DefaultTransport should be untouched when --gateway-insecure is not set")
	}
	if got := buf.String(); got != "" {
		t.Errorf("stderr should be empty when --gateway-insecure is not set, got %q", got)
	}
}

// TestNoPersistentPreRunEOverrideWithoutChaining walks the cobra command tree
// and fails if any non-root command defines PersistentPreRunE. Root's
// PersistentPreRunE emits the --gateway-insecure warning and calls
// applyGatewayInsecureTransport(); cobra silently skips root's hook for any
// subcommand that defines its own — so new subcommands must either not define
// PersistentPreRunE, or explicitly chain the parent's. This test makes that
// constraint visible before it bites in production.
func TestNoPersistentPreRunEOverrideWithoutChaining(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	root := NewRootCommand()

	var check func(cmd *cobra.Command, depth int)
	check = func(cmd *cobra.Command, depth int) {
		if depth > 0 && cmd.PersistentPreRunE != nil {
			t.Errorf("command %q defines PersistentPreRunE without chaining root's hook; --gateway-insecure warning and transport override would be skipped for this subtree", cmd.CommandPath())
		}
		for _, child := range cmd.Commands() {
			check(child, depth+1)
		}
	}
	check(root, 0)
}
