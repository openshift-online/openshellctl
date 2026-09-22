package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestCreate_EditorUnsupportedExit2(t *testing.T) {
	_, err := runCmd(t, "sandbox", "create", "--editor", "vscode")
	if err == nil {
		t.Fatal("expected error for --editor")
	}
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want %d", exitCodeFor(err), ExitUsage)
	}
	if !strings.Contains(err.Error(), "--editor is not supported") {
		t.Errorf("msg = %v", err)
	}
}

func TestCreate_DockerfileFromExit2(t *testing.T) {
	dir := t.TempDir()
	df := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(df, []byte("FROM x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runCmd(t, "sandbox", "create", "--from", df)
	if err == nil {
		t.Fatal("expected error for Dockerfile --from")
	}
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want %d (usage)", exitCodeFor(err), ExitUsage)
	}
}

func TestCreate_BadEnvExit2(t *testing.T) {
	_, err := runCmd(t, "sandbox", "create", "--env", "1BAD=x", "--gateway-endpoint", "http://localhost:1")
	if err == nil {
		t.Fatal("expected error for bad env key")
	}
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want %d", exitCodeFor(err), ExitUsage)
	}
}

func TestDelete_NoNameNoAllExit2(t *testing.T) {
	_, err := runCmd(t, "sandbox", "delete")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("err = %v exit = %d, want usage", err, exitCodeFor(err))
	}
}

func TestDelete_AllWithNameExit2(t *testing.T) {
	_, err := runCmd(t, "sandbox", "delete", "--all", "somename")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("err = %v exit = %d, want usage", err, exitCodeFor(err))
	}
}

func TestDelete_FileExtractsName(t *testing.T) {
	manifest := `apiVersion: openshell.managed.openshift.io/v1alpha1
kind: Sandbox
metadata:
  name: from-file
spec:
  image: test:latest
`
	dir := t.TempDir()
	path := filepath.Join(dir, "sb.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	// delete -f should parse the name but fail on gateway (no connection) —
	// the key assertion is that it does NOT fail with "a sandbox name is required".
	_, err := runCmd(t, "sandbox", "delete", "--file", path)
	if err == nil {
		t.Fatal("expected gateway error, not success")
	}
	if exitCodeFor(err) == ExitUsage {
		t.Fatalf("should not be a usage error — -f should provide the name: %v", err)
	}
}

func TestDelete_FileNoName(t *testing.T) {
	manifest := `apiVersion: openshell.managed.openshift.io/v1alpha1
kind: Sandbox
metadata: {}
spec:
  image: test:latest
`
	dir := t.TempDir()
	path := filepath.Join(dir, "sb.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runCmd(t, "sandbox", "delete", "--file", path)
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("err = %v exit = %d, want usage (no name)", err, exitCodeFor(err))
	}
}

func TestGet_NoNameExit2(t *testing.T) {
	_, err := runCmd(t, "sandbox", "get")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("err = %v exit = %d, want usage", err, exitCodeFor(err))
	}
}

func TestList_IdsAndNamesMutuallyExclusive(t *testing.T) {
	_, err := runCmd(t, "sandbox", "list", "--ids", "--names")
	if err == nil {
		t.Fatal("expected mutual-exclusion error")
	}
	if exitCodeFor(err) != ExitUsage {
		t.Errorf("exit = %d, want usage", exitCodeFor(err))
	}
}

func TestPolicyLint_Clean(t *testing.T) {
	dir := t.TempDir()
	pf := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(pf, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "policy", "lint", pf)
	if err != nil {
		t.Fatalf("lint clean: %v", err)
	}
	if !strings.Contains(out, "no problems found") {
		t.Errorf("out = %q", out)
	}
}

func TestPolicyLint_Findings(t *testing.T) {
	dir := t.TempDir()
	pf := filepath.Join(dir, "policy.yaml")
	body := "version: 1\nprocess:\n  run_as_user: \"0\"\n"
	if err := os.WriteFile(pf, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "policy", "lint", pf)
	if err == nil {
		t.Fatal("expected lint findings to error")
	}
	if !strings.Contains(out, "run_as_user must be 'sandbox'") {
		t.Errorf("out = %q", out)
	}
}

func TestPolicyLint_ParseError(t *testing.T) {
	dir := t.TempDir()
	pf := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(pf, []byte("version: 1\nbogus: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runCmd(t, "policy", "lint", pf)
	if err == nil {
		t.Fatal("expected parse error for unknown field")
	}
}

// testManifest is a minimal valid manifest with metadata.name set.
const testManifest = `apiVersion: openshell.managed.openshift.io/v1alpha1
kind: Sandbox
metadata:
  name: from-file
spec:
  image: test:latest
`

func writeManifest(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sb.yaml")
	body := strings.ReplaceAll(testManifest, "from-file", name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- -f flag tests across subcommands ---
// These verify that -f provides the sandbox name (getting past the "name required"
// check) and that -f + positional name is rejected as a usage error.

func TestConnect_FileFlag(t *testing.T) {
	path := writeManifest(t, "my-sb")
	// -f should provide the name; command will fail on gateway (no connection),
	// NOT on "a sandbox name is required".
	_, err := runCmd(t, "sandbox", "connect", "--file", path)
	if err == nil {
		t.Fatal("expected gateway error, not success")
	}
	if exitCodeFor(err) == ExitUsage {
		t.Fatalf("-f should provide the name, got usage error: %v", err)
	}
}

func TestConnect_FilePlusPositionalConflict(t *testing.T) {
	path := writeManifest(t, "my-sb")
	_, err := runCmd(t, "sandbox", "connect", "--file", path, "other-name")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("expected usage error for -f + positional, got %v (exit %d)", err, exitCodeFor(err))
	}
}

func TestExec_FileFlag(t *testing.T) {
	path := writeManifest(t, "my-sb")
	_, err := runCmd(t, "sandbox", "exec", "--file", path, "--", "echo", "hi")
	if err == nil {
		t.Fatal("expected gateway error, not success")
	}
	if exitCodeFor(err) == ExitUsage {
		t.Fatalf("-f should provide the name, got usage error: %v", err)
	}
}

func TestExec_FilePlusNameConflict(t *testing.T) {
	path := writeManifest(t, "my-sb")
	_, err := runCmd(t, "sandbox", "exec", "--file", path, "--name", "other", "--", "echo", "hi")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("expected usage error for -f + --name, got %v (exit %d)", err, exitCodeFor(err))
	}
}

func TestGet_FileFlag(t *testing.T) {
	path := writeManifest(t, "my-sb")
	_, err := runCmd(t, "sandbox", "get", "--file", path)
	if err == nil {
		t.Fatal("expected gateway error, not success")
	}
	if exitCodeFor(err) == ExitUsage {
		t.Fatalf("-f should provide the name, got usage error: %v", err)
	}
}

func TestGet_FilePlusPositionalConflict(t *testing.T) {
	path := writeManifest(t, "my-sb")
	_, err := runCmd(t, "sandbox", "get", "--file", path, "other-name")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("expected usage error for -f + positional, got %v (exit %d)", err, exitCodeFor(err))
	}
}

func TestStop_FileFlag(t *testing.T) {
	path := writeManifest(t, "my-sb")
	_, err := runCmd(t, "sandbox", "stop", "--file", path)
	if err == nil {
		t.Fatal("expected gateway error, not success")
	}
	if exitCodeFor(err) == ExitUsage {
		t.Fatalf("-f should provide the name, got usage error: %v", err)
	}
}

func TestStop_FilePlusPositionalConflict(t *testing.T) {
	path := writeManifest(t, "my-sb")
	_, err := runCmd(t, "sandbox", "stop", "--file", path, "other-name")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("expected usage error for -f + positional, got %v (exit %d)", err, exitCodeFor(err))
	}
}

func TestStart_FileFlag(t *testing.T) {
	path := writeManifest(t, "my-sb")
	_, err := runCmd(t, "sandbox", "start", "--file", path)
	if err == nil {
		t.Fatal("expected gateway error, not success")
	}
	if exitCodeFor(err) == ExitUsage {
		t.Fatalf("-f should provide the name, got usage error: %v", err)
	}
}

func TestStart_FilePlusPositionalConflict(t *testing.T) {
	path := writeManifest(t, "my-sb")
	_, err := runCmd(t, "sandbox", "start", "--file", path, "other-name")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("expected usage error for -f + positional, got %v (exit %d)", err, exitCodeFor(err))
	}
}

func TestLogs_FileFlag(t *testing.T) {
	path := writeManifest(t, "my-sb")
	// logs is a top-level command, not under sandbox
	_, err := runCmd(t, "logs", "--file", path)
	if err == nil {
		t.Fatal("expected gateway error, not success")
	}
	if exitCodeFor(err) == ExitUsage {
		t.Fatalf("-f should provide the name, got usage error: %v", err)
	}
}

func TestSSHConfig_FileFlag(t *testing.T) {
	path := writeManifest(t, "my-sb")
	// ssh-config doesn't need a gateway connection — it just prints the config
	// block. So it succeeds when -f provides the name.
	out, err := runCmd(t, "sandbox", "ssh-config", "--file", path)
	if err != nil {
		t.Fatalf("ssh-config -f should succeed, got: %v", err)
	}
	if !strings.Contains(out, "Host openshell-my-sb.") {
		t.Errorf("expected ssh config with sandbox name, got: %s", out)
	}
}

// --- Upload/download argument format tests ---
// New format: upload NAME LOCAL_PATH [DEST]  /  download NAME SANDBOX_PATH [DEST]

func TestUpload_NewArgFormat(t *testing.T) {
	// upload NAME LOCAL_PATH — should get past argument validation.
	// Will fail on gateway, NOT on usage.
	_, err := runCmd(t, "sandbox", "upload", "my-sb", "/tmp/somefile")
	if err == nil {
		t.Fatal("expected gateway error, not success")
	}
	if exitCodeFor(err) == ExitUsage {
		t.Fatalf("expected gateway error, got usage error: %v", err)
	}
}

func TestUpload_NewArgFormatWithDest(t *testing.T) {
	_, err := runCmd(t, "sandbox", "upload", "my-sb", "/tmp/somefile", "/remote/dest")
	if err == nil {
		t.Fatal("expected gateway error, not success")
	}
	if exitCodeFor(err) == ExitUsage {
		t.Fatalf("expected gateway error, got usage error: %v", err)
	}
}

func TestUpload_FileFlag(t *testing.T) {
	path := writeManifest(t, "my-sb")
	_, err := runCmd(t, "sandbox", "upload", "--file", path, "/tmp/somefile")
	if err == nil {
		t.Fatal("expected gateway error, not success")
	}
	if exitCodeFor(err) == ExitUsage {
		t.Fatalf("-f should provide the name, got usage error: %v", err)
	}
}

func TestUpload_FilePlusTooManyArgs(t *testing.T) {
	path := writeManifest(t, "my-sb")
	// With -f, args should be LOCAL_PATH [DEST] (1-2 args). 3 args is an error.
	_, err := runCmd(t, "sandbox", "upload", "--file", path, "name", "/tmp/file", "/remote")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("expected usage error for -f + 3 positional args, got %v (exit %d)", err, exitCodeFor(err))
	}
}

func TestUpload_TooFewArgs(t *testing.T) {
	_, err := runCmd(t, "sandbox", "upload")
	if err == nil {
		t.Fatal("expected error for no args")
	}
}

func TestDownload_NewArgFormat(t *testing.T) {
	_, err := runCmd(t, "sandbox", "download", "my-sb", "/sandbox/file")
	if err == nil {
		t.Fatal("expected gateway error, not success")
	}
	if exitCodeFor(err) == ExitUsage {
		t.Fatalf("expected gateway error, got usage error: %v", err)
	}
}

func TestDownload_NewArgFormatWithDest(t *testing.T) {
	_, err := runCmd(t, "sandbox", "download", "my-sb", "/sandbox/file", "/tmp/local")
	if err == nil {
		t.Fatal("expected gateway error, not success")
	}
	if exitCodeFor(err) == ExitUsage {
		t.Fatalf("expected gateway error, got usage error: %v", err)
	}
}

func TestDownload_FileFlag(t *testing.T) {
	path := writeManifest(t, "my-sb")
	_, err := runCmd(t, "sandbox", "download", "--file", path, "/sandbox/file")
	if err == nil {
		t.Fatal("expected gateway error, not success")
	}
	if exitCodeFor(err) == ExitUsage {
		t.Fatalf("-f should provide the name, got usage error: %v", err)
	}
}

func TestDownload_TooFewArgs(t *testing.T) {
	_, err := runCmd(t, "sandbox", "download")
	if err == nil {
		t.Fatal("expected error for no args")
	}
}

func TestEnvSecondsDefault(t *testing.T) {
	t.Setenv("OPENSHELL_LIFECYCLE_TIMEOUT", "42")
	if got := lifecycleTimeout(); got.Seconds() != 42 {
		t.Errorf("lifecycleTimeout = %v, want 42s", got)
	}
	t.Setenv("OPENSHELL_LIFECYCLE_TIMEOUT", "")
	if got := lifecycleTimeout(); got.Seconds() != 300 {
		t.Errorf("default = %v, want 300s", got)
	}
}
