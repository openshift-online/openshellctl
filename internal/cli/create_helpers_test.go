package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/spf13/cobra"
)

func TestLoadManifest_FileAndStdin(t *testing.T) {
	manifest := `apiVersion: openshell.managed.openshift.io/v1alpha1
kind: Sandbox
metadata:
  name: sb-1
spec:
  image: ghcr.io/x/y:latest
`
	dir := t.TempDir()
	path := filepath.Join(dir, "m.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	m, err := loadManifest(cmd, path)
	if err != nil {
		t.Fatalf("loadManifest(file): %v", err)
	}
	if m.Metadata.Name != "sb-1" {
		t.Errorf("name = %q", m.Metadata.Name)
	}

	// Stdin path.
	cmd2 := &cobra.Command{}
	cmd2.SetIn(strings.NewReader(manifest))
	m2, err := loadManifest(cmd2, "-")
	if err != nil {
		t.Fatalf("loadManifest(stdin): %v", err)
	}
	if m2.Spec.Image != "ghcr.io/x/y:latest" {
		t.Errorf("image = %q", m2.Spec.Image)
	}
}

func TestLoadManifest_InvalidReturnsUsage(t *testing.T) {
	manifest := `apiVersion: openshell.managed.openshift.io/v1alpha1
kind: Sandbox
metadata:
  name: sb
spec:
  approvalMode: bogus
`
	dir := t.TempDir()
	path := filepath.Join(dir, "m.yaml")
	_ = os.WriteFile(path, []byte(manifest), 0o600)
	_, err := loadManifest(&cobra.Command{}, path)
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("err = %v exit = %d, want usage", err, exitCodeFor(err))
	}
}

func TestBuildCreateFlags_ImageResolution(t *testing.T) {
	f, err := buildCreateFlags(&cobra.Command{}, createFlagInput{from: "python"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Image != "ghcr.io/nvidia/openshell-community/sandboxes/python:latest" {
		t.Errorf("image = %q", f.Image)
	}
}

func TestBuildCreateFlags_BadLabel(t *testing.T) {
	_, err := buildCreateFlags(&cobra.Command{}, createFlagInput{labels: []string{"noequals"}})
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("err = %v exit = %d, want usage", err, exitCodeFor(err))
	}
}

func TestRenderSandbox_Formats(t *testing.T) {
	s := &types.Sandbox{ID: "id", Name: "sb", Status: types.SandboxStatus{Phase: types.SandboxReady}}
	for _, format := range []string{"table", "json", "yaml"} {
		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := renderSandbox(cmd, s, format); err != nil {
			t.Fatalf("renderSandbox(%s): %v", format, err)
		}
		if !strings.Contains(out.String(), "sb") {
			t.Errorf("%s output missing name: %q", format, out.String())
		}
	}
}

func TestRenderSandboxSlice(t *testing.T) {
	sandboxes := []*types.Sandbox{
		{Name: "a", Status: types.SandboxStatus{Phase: types.SandboxReady}},
		{Name: "b", Status: types.SandboxStatus{Phase: types.SandboxReady}},
	}
	cmd := &cobra.Command{}
	var j bytes.Buffer
	cmd.SetOut(&j)
	if err := renderSandboxSliceJSON(cmd, sandboxes); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(j.String(), `"a"`) || !strings.Contains(j.String(), `"b"`) {
		t.Errorf("json slice = %q", j.String())
	}

	cmd2 := &cobra.Command{}
	var y bytes.Buffer
	cmd2.SetOut(&y)
	if err := renderSandboxSliceYAML(cmd2, sandboxes); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(y.String(), "name: a") {
		t.Errorf("yaml slice = %q", y.String())
	}
}

func TestCommandArgs(t *testing.T) {
	// Build a command that records ArgsLenAtDash via parsing.
	cmd := &cobra.Command{Use: "create", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().String("name", "", "")
	cmd.SetArgs([]string{"--name", "x", "--", "echo", "hi"})
	var got []string
	cmd.RunE = func(c *cobra.Command, args []string) error {
		got = commandArgs(c, args)
		return nil
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "echo" || got[1] != "hi" {
		t.Errorf("commandArgs = %v, want [echo hi]", got)
	}
}
