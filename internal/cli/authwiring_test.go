package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestClientSecretProvider_None(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	if got := clientSecretProvider(""); got != nil {
		t.Error("expected nil provider when no secret configured")
	}
}

func TestClientSecretProvider_FromFile(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("  s3cr3t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := clientSecretProvider(path)
	if p == nil {
		t.Fatal("expected a provider for a file path")
	}
	got, err := p(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "s3cr3t" {
		t.Errorf("secret = %q, want trimmed s3cr3t", got)
	}
}

func TestClientSecretProvider_EnvWins(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetEnvPrefix("OPENSHELL")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(envKeyReplacer())
	t.Setenv("OPENSHELL_OIDC_CLIENT_SECRET", "from-env")

	p := clientSecretProvider("/nonexistent/file")
	if p == nil {
		t.Fatal("expected a provider from env")
	}
	got, err := p(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-env" {
		t.Errorf("secret = %q, want from-env", got)
	}
}
