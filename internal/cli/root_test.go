package cli

import (
	"context"
	"net/http"
	"testing"

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

// TestGatewayInsecureFlag_OverridesDefaultTransport confirms --gateway-insecure
// swaps http.DefaultTransport to one that skips TLS verification. This is the
// only lever available to make the OpenShell SDK's internal OIDC
// discovery/token HTTP client (an unexported package-level *http.Client with
// no Transport override, hence http.DefaultTransport) respect the flag — the
// SDK exposes no option to inject a custom client or skip verification.
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
}

// TestGatewayInsecureFlag_NotSetLeavesTransportAlone confirms the common case
// (no --gateway-insecure) never touches the global transport.
func TestGatewayInsecureFlag_NotSetLeavesTransportAlone(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })

	root := NewRootCommand()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if http.DefaultTransport != original {
		t.Error("DefaultTransport should be untouched when --gateway-insecure is not set")
	}
}
