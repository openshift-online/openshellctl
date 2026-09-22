package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/openshift-online/openshellctl/internal/version"
	"github.com/spf13/viper"
)

func TestVersionCommand_PrintsPin(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	oldPin := version.OpenShellPin
	t.Cleanup(func() { version.OpenShellPin = oldPin })
	version.OpenShellPin = "v0.0.116"

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("version command errored: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "v0.0.116") {
		t.Errorf("version output missing openshell pin; got:\n%s", got)
	}
	if !strings.Contains(got, "openshell-pin:") {
		t.Errorf("version output missing openshell-pin label; got:\n%s", got)
	}
}
