package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/spf13/cobra"
	"go.uber.org/mock/gomock"

	"github.com/openshift-online/openshellctl/pkg/gateway/mock"
)

// templatedManifest is the motivating example from ROSAENG-74244: a
// hardcoded metadata.name (which collides on reuse) and a per-run PROMPT,
// both pulled out as ${KEY} references.
const templatedManifest = `apiVersion: openshell.managed.openshift.io/v1alpha1
kind: Sandbox
metadata:
  name: ${NAME}
spec:
  image: python
  command:
    - claude
    - --dangerously-skip-permissions
    - '${PROMPT}'
`

func writeManifestFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sandbox.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestSandboxCreate_Param_SubstitutesIntoCommand is the card's first
// acceptance criterion end to end: -p NAME=X -p PROMPT="a, b: c" creates a
// sandbox named X whose command's third element is exactly "a, b: c" — a
// value containing both a comma and a colon, which a StringSlice flag or raw
// text substitution would have corrupted.
func TestSandboxCreate_Param_SubstitutesIntoCommand(t *testing.T) {
	path := writeManifestFile(t, templatedManifest)

	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	var gotCommand []string
	gw.EXPECT().CreateSandbox(gomock.Any(), "default", "X", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ interface{}, _, name string, spec *types.SandboxSpec, _ map[string]string) (*types.Sandbox, error) {
			gotCommand = spec.Command
			return &types.Sandbox{Name: name}, nil
		})

	out, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "sandbox", "create",
		"-f", path, "-p", "NAME=X", "-p", "PROMPT=a, b: c", "--detach", "--no-auto-providers", "-o", "json")
	if err != nil {
		t.Fatalf("sandbox create: %v\noutput:\n%s", err, out)
	}
	if len(gotCommand) != 3 || gotCommand[2] != "a, b: c" {
		t.Errorf("command = %#v, want third element exactly %q", gotCommand, "a, b: c")
	}
}

// TestSandboxCreate_Param_PartialSubstitution_FailsBeforeGatewayCall is the
// card's second acceptance criterion: the same manifest with only -p NAME=X
// fails before any gateway call, naming PROMPT as unresolved, with a
// non-zero exit code.
func TestSandboxCreate_Param_PartialSubstitution_FailsBeforeGatewayCall(t *testing.T) {
	path := writeManifestFile(t, templatedManifest)

	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	// No CreateSandbox expectation — gomock fails the test if it's called.

	_, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "sandbox", "create",
		"-f", path, "-p", "NAME=X", "--detach")
	if err == nil {
		t.Fatal("expected an error")
	}
	if exitCodeFor(err) == ExitOK {
		t.Error("exit = 0, want non-zero")
	}
	if !strings.Contains(err.Error(), "PROMPT") {
		t.Errorf("error should name PROMPT as unresolved, got: %v", err)
	}
}

// TestSandboxCreate_Param_UnknownParameter_Errors is the card's third
// acceptance criterion: -p EXTRA=1 against a manifest that does not
// reference EXTRA fails unless --ignore-unknown-parameters is set.
func TestSandboxCreate_Param_UnknownParameter_Errors(t *testing.T) {
	path := writeManifestFile(t, "apiVersion: openshell.managed.openshift.io/v1alpha1\nkind: Sandbox\nmetadata:\n  name: sb\nspec:\n  image: python\n")

	_, err := runCmd(t, "sandbox", "create", "-f", path, "-p", "EXTRA=1", "--detach")
	if err == nil {
		t.Fatal("expected an error")
	}
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want ExitUsage (2)", exitCodeFor(err))
	}
	if !strings.Contains(err.Error(), "EXTRA") {
		t.Errorf("error should name EXTRA, got: %v", err)
	}

	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().CreateSandbox(gomock.Any(), "default", "sb", gomock.Any(), gomock.Any()).
		Return(&types.Sandbox{Name: "sb"}, nil)
	out, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "sandbox", "create",
		"-f", path, "-p", "EXTRA=1", "--ignore-unknown-parameters", "--detach", "-o", "json")
	if err != nil {
		t.Fatalf("with --ignore-unknown-parameters: %v\noutput:\n%s", err, out)
	}
}

// TestSandboxCreate_Parameters_PrintsNamesAndExits is the card's fourth
// acceptance criterion: --parameters -f sandbox.yaml prints NAME and PROMPT,
// each on its own line, and makes no gateway call.
func TestSandboxCreate_Parameters_PrintsNamesAndExits(t *testing.T) {
	path := writeManifestFile(t, templatedManifest)

	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	// No expectations set — any call fails the test.

	out, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "sandbox", "create", "-f", path, "--parameters")
	if err != nil {
		t.Fatalf("--parameters: %v\noutput:\n%s", err, out)
	}
	if strings.TrimSpace(out) != "NAME\nPROMPT" {
		t.Errorf("output = %q, want \"NAME\\nPROMPT\"", out)
	}
}

// TestSandboxCreate_ParamFile_OverriddenByParam_WarnsOnStderr is the card's
// fifth acceptance criterion: --param-file values are overridden by --param
// for the same key, with a stderr warning.
func TestSandboxCreate_ParamFile_OverriddenByParam_WarnsOnStderr(t *testing.T) {
	manifestPath := writeManifestFile(t, "apiVersion: openshell.managed.openshift.io/v1alpha1\nkind: Sandbox\nmetadata:\n  name: ${NAME}\nspec:\n  image: python\n")
	dir := t.TempDir()
	paramFile := filepath.Join(dir, "params.txt")
	if err := os.WriteFile(paramFile, []byte("NAME=fromfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().CreateSandbox(gomock.Any(), "default", "fromflag", gomock.Any(), gomock.Any()).
		Return(&types.Sandbox{Name: "fromflag"}, nil)

	out, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "sandbox", "create",
		"-f", manifestPath, "--param-file", paramFile, "-p", "NAME=fromflag", "--detach", "-o", "json")
	if err != nil {
		t.Fatalf("sandbox create: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "Warning: --param NAME overrides value from --param-file") {
		t.Errorf("expected an override warning on stderr, got:\n%s", out)
	}
}

// TestSandboxCreate_NoReferences_IdenticalToUntemplatedDecode is the card's
// sixth acceptance criterion: a manifest with no ${...} references, and no
// -p flags, decodes identically whether or not the templating machinery
// exists — loadManifestWithParams must skip the YAML round-trip entirely in
// this case (not merely produce an equal-by-chance result), since
// yaml.v3's Marshal reformats indentation and would otherwise silently
// change behavior for every manifest that never uses this feature.
func TestSandboxCreate_NoReferences_IdenticalToUntemplatedDecode(t *testing.T) {
	const manifest = "apiVersion: openshell.managed.openshift.io/v1alpha1\n" +
		"kind: Sandbox\n" +
		"metadata:\n  name: sb\n" +
		"spec:\n  image: python\n"
	path := writeManifestFile(t, manifest)

	direct, err := decodeManifestBytes([]byte(manifest))
	if err != nil {
		t.Fatalf("decodeManifestBytes: %v", err)
	}

	cmd := &cobra.Command{}
	viaLoader, printedOnly, err := loadManifestWithParams(cmd, osFS{}, path, templateFlags{})
	if err != nil {
		t.Fatalf("loadManifestWithParams: %v", err)
	}
	if printedOnly {
		t.Fatal("printedOnly = true, want false")
	}
	if viaLoader.Metadata.Name != direct.Metadata.Name || viaLoader.Spec.Image != direct.Spec.Image {
		t.Errorf("loadManifestWithParams() = %+v, want identical to decodeManifestBytes() = %+v", viaLoader, direct)
	}
}

// TestSandboxCreate_ParamWithoutFile_UsageError confirms --param/--param-file/
// --parameters/--ignore-unknown-parameters require -f, since there is no
// manifest to substitute into otherwise.
func TestSandboxCreate_ParamWithoutFile_UsageError(t *testing.T) {
	_, err := runCmd(t, "sandbox", "create", "--from", "python", "-p", "NAME=X", "--detach")
	if err == nil {
		t.Fatal("expected a usage error")
	}
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want ExitUsage (2)", exitCodeFor(err))
	}
	if !strings.Contains(err.Error(), "-f") && !strings.Contains(err.Error(), "--file") {
		t.Errorf("error should mention -f/--file, got: %v", err)
	}
}

// TestSandboxCreate_MalformedParam_UsageError confirms a malformed --param
// value (no '=') surfaces as a usage error naming the offending text.
func TestSandboxCreate_MalformedParam_UsageError(t *testing.T) {
	path := writeManifestFile(t, templatedManifest)
	_, err := runCmd(t, "sandbox", "create", "-f", path, "-p", "NOTKEYVALUE", "--detach")
	if err == nil {
		t.Fatal("expected a usage error")
	}
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want ExitUsage (2)", exitCodeFor(err))
	}
	if !strings.Contains(err.Error(), "NOTKEYVALUE") {
		t.Errorf("error should mention the offending text, got: %v", err)
	}
}

// TestSandboxCreate_ParamFile_UnreadablePath_UsageError confirms a missing
// --param-file path surfaces as a usage error rather than a raw os.Open
// failure reaching the user unwrapped.
func TestSandboxCreate_ParamFile_UnreadablePath_UsageError(t *testing.T) {
	path := writeManifestFile(t, templatedManifest)
	_, err := runCmd(t, "sandbox", "create", "-f", path, "--param-file", "/no/such/file.txt", "--detach")
	if err == nil {
		t.Fatal("expected a usage error")
	}
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want ExitUsage (2)", exitCodeFor(err))
	}
}

// TestApplyManifestTemplate_UsesInjectedFS confirms the orchestrator reads
// --param-file through the fs.FS it is given (a seam the CLI's production
// osFS production path never exercises in-test) rather than the real
// filesystem.
func TestApplyManifestTemplate_UsesInjectedFS(t *testing.T) {
	fsys := fstest.MapFS{
		"params.txt": {Data: []byte("NAME=fromfs\n")},
	}
	var out, errw strings.Builder
	substituted, printedOnly, err := applyManifestTemplate(fsys, &out, &errw,
		[]byte("metadata:\n  name: ${NAME}\n"),
		templateFlags{paramFiles: []string{"params.txt"}})
	if err != nil {
		t.Fatalf("applyManifestTemplate: %v", err)
	}
	if printedOnly {
		t.Fatal("printedOnly = true, want false")
	}
	if !strings.Contains(string(substituted), "fromfs") {
		t.Errorf("substituted manifest = %q, want it to contain %q", substituted, "fromfs")
	}
}
