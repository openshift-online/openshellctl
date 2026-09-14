package cli

import (
	"testing"
	"testing/fstest"

	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
	"github.com/openshift-online/openshellctl/pkg/sandbox"
)

func targetWithLastSandbox(ws, name string) *gatewayconfig.Target {
	return &gatewayconfig.Target{
		Name: "ci",
		Resolved: &gatewayconfig.Resolved{
			Name: "ci",
			FS:   fstest.MapFS{"last_sandbox": {Data: []byte(ws + "\n" + name)}},
		},
	}
}

func TestResolveSandboxName(t *testing.T) {
	target := targetWithLastSandbox("default", "remembered")

	// Explicit arg wins.
	if got, err := resolveSandboxName([]string{"explicit"}, target, "default"); err != nil || got != "explicit" {
		t.Errorf("explicit arg: got %q err %v", got, err)
	}

	// No arg → falls back to last_sandbox for the workspace.
	if got, err := resolveSandboxName(nil, target, "default"); err != nil || got != "remembered" {
		t.Errorf("last_sandbox: got %q err %v", got, err)
	}

	// No arg, workspace mismatch → not found → usage error.
	_, err := resolveSandboxName(nil, target, "other-ws")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Errorf("workspace mismatch: err %v exit %d, want usage", err, exitCodeFor(err))
	}

	// No arg, no target → usage error.
	_, err = resolveSandboxName(nil, nil, "default")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Errorf("no target: err %v exit %d, want usage", err, exitCodeFor(err))
	}
}

func TestSaveLastSandboxRoundTrip(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	target := &gatewayconfig.Target{Name: "ci"}
	saveLastSandbox(target, "default", "my-sb")

	// Read it back via the gatewayconfig loader against the written tree.
	env, err := gatewayconfig.NewOSEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	// Write the minimal metadata the loader needs, then reload.
	got, err := gatewayconfig.NewOSWriter()
	if err != nil {
		t.Fatalf("writer: %v", err)
	}
	_ = got.WriteFile(gatewayconfig.GatewayDir("ci")+"/metadata.json", []byte(`{"endpoint":"http://x"}`), 0o600)

	resolved, err := gatewayconfig.Load(env, "ci")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	name, ok := gatewayconfig.LoadLastSandbox(resolved, "default")
	if !ok || name != "my-sb" {
		t.Errorf("round-trip last_sandbox = %q ok=%v, want my-sb", name, ok)
	}
}

func TestSaveLastSandboxNoTargetNoop(t *testing.T) {
	// No panic and no write when target/name is empty.
	saveLastSandbox(nil, "default", "x")
	saveLastSandbox(&gatewayconfig.Target{}, "default", "x")
}

func TestRemoteExitErrorMessage(t *testing.T) {
	e := &RemoteExitError{Code: 5}
	if e.Error() != "remote process exited with status 5" {
		t.Errorf("msg = %q", e.Error())
	}
}

func TestResolveTTYHelper(t *testing.T) {
	yes := true
	if !resolveTTY(&sandbox.CreateRequest{TTY: &yes}) {
		t.Error("explicit tty true should resolve true")
	}
	if resolveTTY(&sandbox.CreateRequest{}) {
		t.Error("default create tty should be false")
	}
}

func TestValidateCreateRequest(t *testing.T) {
	if err := validateCreateRequest(&sandbox.CreateRequest{CPU: "abc"}); err == nil || exitCodeFor(err) != ExitUsage {
		t.Errorf("bad cpu: err %v exit %d, want usage", err, exitCodeFor(err))
	}
	if err := validateCreateRequest(&sandbox.CreateRequest{Memory: "notmem"}); err == nil || exitCodeFor(err) != ExitUsage {
		t.Errorf("bad memory: err %v", err)
	}
	if err := validateCreateRequest(&sandbox.CreateRequest{CPU: "500m", Memory: "1Gi"}); err != nil {
		t.Errorf("valid cpu/memory: %v", err)
	}
}
