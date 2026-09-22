package cli

import (
	"context"
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
