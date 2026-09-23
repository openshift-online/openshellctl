package sandbox

import (
	"testing"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/openshift-online/openshellctl/pkg/api/v1alpha1"
)

func TestMerge_FlagWinsOverManifest(t *testing.T) {
	m := &v1alpha1.Sandbox{
		Metadata: v1alpha1.ObjectMeta{Name: "from-manifest", Workspace: "ws1", Labels: map[string]string{"a": "1"}},
		Spec:     v1alpha1.SandboxSpec{Image: "img-manifest", Env: map[string]string{"X": "m"}},
	}
	f := CreateFlags{
		Name:   "from-flag",
		Image:  "img-flag",
		Labels: map[string]string{"b": "2"},
		Env:    map[string]string{"X": "f", "Y": "y"},
	}
	r, err := MergeManifestAndFlags(m, f)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "from-flag" || r.Image != "img-flag" {
		t.Errorf("flag should win: %+v", r)
	}
	if r.Workspace != "ws1" {
		t.Errorf("workspace from manifest = %q", r.Workspace)
	}
	// Maps merge, flag key wins.
	if r.Labels["a"] != "1" || r.Labels["b"] != "2" {
		t.Errorf("labels merge = %v", r.Labels)
	}
	if r.Env["X"] != "f" || r.Env["Y"] != "y" {
		t.Errorf("env merge (flag wins) = %v", r.Env)
	}
}

func TestMerge_Defaults(t *testing.T) {
	r, err := MergeManifestAndFlags(nil, CreateFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Workspace != "default" {
		t.Errorf("workspace default = %q", r.Workspace)
	}
	if !r.Keep {
		t.Error("keep should default true")
	}
	if r.Output != "table" {
		t.Errorf("output default = %q", r.Output)
	}
}

func TestMerge_NoKeep(t *testing.T) {
	no := false
	r, _ := MergeManifestAndFlags(nil, CreateFlags{Keep: &no})
	if r.Keep {
		t.Error("--no-keep should set Keep=false")
	}
}

func TestMerge_ListReplaceWhenFlagNonEmpty(t *testing.T) {
	m := &v1alpha1.Sandbox{Spec: v1alpha1.SandboxSpec{Command: []string{"manifest-cmd"}}}
	// No command flag → manifest kept.
	r, _ := MergeManifestAndFlags(m, CreateFlags{})
	if len(r.Command) != 1 || r.Command[0] != "manifest-cmd" {
		t.Errorf("command = %v", r.Command)
	}
	// Command flag → replaces.
	r2, _ := MergeManifestAndFlags(m, CreateFlags{Command: []string{"flag-cmd", "arg"}})
	if len(r2.Command) != 2 || r2.Command[0] != "flag-cmd" {
		t.Errorf("command not replaced: %v", r2.Command)
	}
}

func TestMerge_SessionOpts(t *testing.T) {
	m := &v1alpha1.Sandbox{
		Spec: v1alpha1.SandboxSpec{
			Image: "img",
			SessionOpts: &v1alpha1.SessionOpts{
				NoKeep:       true,
				Detach:       true,
				ApprovalMode: "auto",
				Output:       "json",
				Forward:      "8080",
			},
		},
	}
	r, err := MergeManifestAndFlags(m, CreateFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Keep {
		t.Error("sessionOpts.noKeep should set Keep=false")
	}
	if !r.Detach {
		t.Error("sessionOpts.detach should be true")
	}
	if r.ApprovalMode != "auto" {
		t.Errorf("approvalMode = %q, want auto", r.ApprovalMode)
	}
	if r.Output != "json" {
		t.Errorf("output = %q, want json", r.Output)
	}
	if r.Forward == nil || r.Forward.Port != 8080 {
		t.Errorf("forward = %+v, want port 8080", r.Forward)
	}
}

func TestMerge_SessionOptsOverridesDeprecated(t *testing.T) {
	tru := true
	m := &v1alpha1.Sandbox{
		Spec: v1alpha1.SandboxSpec{
			Image:        "img",
			ApprovalMode: "manual",
			Keep:         &tru,
			SessionOpts: &v1alpha1.SessionOpts{
				NoKeep:       true,
				ApprovalMode: "auto",
			},
		},
	}
	r, err := MergeManifestAndFlags(m, CreateFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Keep {
		t.Error("sessionOpts.noKeep should override deprecated keep")
	}
	if r.ApprovalMode != "auto" {
		t.Errorf("sessionOpts.approvalMode should override deprecated: got %q", r.ApprovalMode)
	}
}

func TestMerge_SessionOptsForwardBad(t *testing.T) {
	m := &v1alpha1.Sandbox{
		Spec: v1alpha1.SandboxSpec{
			SessionOpts: &v1alpha1.SessionOpts{Forward: "notaport"},
		},
	}
	_, err := MergeManifestAndFlags(m, CreateFlags{})
	if err == nil {
		t.Fatal("expected error for bad forward spec")
	}
}

func TestMerge_FlagsOverrideSessionOpts(t *testing.T) {
	m := &v1alpha1.Sandbox{
		Spec: v1alpha1.SandboxSpec{
			Image: "img",
			SessionOpts: &v1alpha1.SessionOpts{
				NoKeep:       true,
				ApprovalMode: "auto",
				Output:       "json",
			},
		},
	}
	yes := true
	r, err := MergeManifestAndFlags(m, CreateFlags{
		Keep:         &yes,
		ApprovalMode: "manual",
		Output:       "yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Keep {
		t.Error("flag --keep should override sessionOpts.noKeep")
	}
	if r.ApprovalMode != "manual" {
		t.Errorf("flag --approval-mode should override: got %q", r.ApprovalMode)
	}
	if r.Output != "yaml" {
		t.Errorf("flag --output should override: got %q", r.Output)
	}
}

func TestMerge_DeprecatedFlatForward(t *testing.T) {
	m := &v1alpha1.Sandbox{
		Spec: v1alpha1.SandboxSpec{
			Image:   "img",
			Forward: "127.0.0.1:3000",
		},
	}
	r, err := MergeManifestAndFlags(m, CreateFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Forward == nil || r.Forward.Port != 3000 || r.Forward.Bind != "127.0.0.1" {
		t.Errorf("deprecated forward = %+v", r.Forward)
	}
}

func TestMerge_DeprecatedFlatForwardBad(t *testing.T) {
	m := &v1alpha1.Sandbox{
		Spec: v1alpha1.SandboxSpec{
			Forward: "badport",
		},
	}
	_, err := MergeManifestAndFlags(m, CreateFlags{})
	if err == nil {
		t.Fatal("expected error for bad deprecated forward")
	}
}

func TestMerge_DeprecatedKeepFalse(t *testing.T) {
	no := false
	m := &v1alpha1.Sandbox{
		Spec: v1alpha1.SandboxSpec{
			Image: "img",
			Keep:  &no,
		},
	}
	r, err := MergeManifestAndFlags(m, CreateFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Keep {
		t.Error("deprecated keep=false should set Keep=false")
	}
}

func TestMerge_SessionOptsForwardOverridesDeprecatedForward(t *testing.T) {
	m := &v1alpha1.Sandbox{
		Spec: v1alpha1.SandboxSpec{
			Image:   "img",
			Forward: "3000",
			SessionOpts: &v1alpha1.SessionOpts{
				Forward: "9090",
			},
		},
	}
	r, err := MergeManifestAndFlags(m, CreateFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Forward == nil || r.Forward.Port != 9090 {
		t.Errorf("sessionOpts.forward should override deprecated: got %+v", r.Forward)
	}
}

func TestMerge_ManifestCommandFlowsThrough(t *testing.T) {
	m := &v1alpha1.Sandbox{
		Spec: v1alpha1.SandboxSpec{
			Image:   "img",
			Command: []string{"claude", "/job-sop"},
		},
	}
	r, err := MergeManifestAndFlags(m, CreateFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Command) != 2 || r.Command[0] != "claude" || r.Command[1] != "/job-sop" {
		t.Errorf("command = %v", r.Command)
	}
}

func TestMerge_NilSessionOptsDefaultsKeepTrue(t *testing.T) {
	m := &v1alpha1.Sandbox{
		Spec: v1alpha1.SandboxSpec{Image: "img"},
	}
	r, err := MergeManifestAndFlags(m, CreateFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Keep {
		t.Error("Keep should default to true when no sessionOpts and no deprecated keep")
	}
}

func TestToSDKSpec_DefaultsCommand(t *testing.T) {
	r := &CreateRequest{}
	spec := ToSDKSpec(r, false)
	if len(spec.Command) != 2 || spec.Command[0] != "/bin/bash" || spec.Command[1] != "-l" {
		t.Errorf("default command = %v", spec.Command)
	}
}

func TestToSDKSpec_TemplateOnlyWhenSet(t *testing.T) {
	// No image/resources/driverconfig → no template.
	if spec := ToSDKSpec(&CreateRequest{}, false); spec.Template != nil {
		t.Error("template should be nil when unset")
	}
	// Image set → template.
	spec := ToSDKSpec(&CreateRequest{Image: "img", CPU: "2"}, false)
	if spec.Template == nil || spec.Template.Image != "img" {
		t.Fatalf("template = %+v", spec.Template)
	}
	limits := spec.Template.Resources["limits"].(map[string]any)
	if limits["cpu"] != "2" {
		t.Errorf("resources = %v", spec.Template.Resources)
	}
}

func TestToSDKSpec_GPUCount(t *testing.T) {
	cnt := uint32(2)
	spec := ToSDKSpec(&CreateRequest{GPU: &v1alpha1.GPU{Count: &cnt}}, false)
	if spec.GPUCount == nil || *spec.GPUCount != 2 {
		t.Errorf("GPUCount = %v", spec.GPUCount)
	}
}

func TestUsesRawGPU(t *testing.T) {
	if (&CreateRequest{}).UsesRawGPU() {
		t.Error("no GPU should not use raw")
	}
	cnt := uint32(1)
	if (&CreateRequest{GPU: &v1alpha1.GPU{Count: &cnt}}).UsesRawGPU() {
		t.Error("explicit count should use SDK, not raw")
	}
	if !(&CreateRequest{GPU: &v1alpha1.GPU{}}).UsesRawGPU() {
		t.Error("bare --gpu (nil count) should use raw")
	}
}

func TestMerge_PolicyFromFlags(t *testing.T) {
	pol := &types.SandboxPolicy{Version: 1, Filesystem: &types.FilesystemPolicy{IncludeWorkdir: true}}
	r, err := MergeManifestAndFlags(nil, CreateFlags{Policy: pol})
	if err != nil {
		t.Fatal(err)
	}
	if r.Policy == nil || r.Policy.Version != 1 {
		t.Errorf("policy not merged from flags: %+v", r.Policy)
	}
	if r.Policy.Filesystem == nil || !r.Policy.Filesystem.IncludeWorkdir {
		t.Error("policy filesystem lost in merge")
	}
}

func TestMerge_FlagPolicyOverridesNilManifest(t *testing.T) {
	m := &v1alpha1.Sandbox{Spec: v1alpha1.SandboxSpec{Image: "img"}}
	pol := &types.SandboxPolicy{Version: 1}
	r, err := MergeManifestAndFlags(m, CreateFlags{Policy: pol})
	if err != nil {
		t.Fatal(err)
	}
	if r.Policy == nil || r.Policy.Version != 1 {
		t.Errorf("flag policy should override nil manifest policy: %+v", r.Policy)
	}
}

func TestToSDKSpec_PolicyFlowsThrough(t *testing.T) {
	pol := &types.SandboxPolicy{
		Version:    1,
		Filesystem: &types.FilesystemPolicy{IncludeWorkdir: true, ReadOnly: []string{"/etc"}},
	}
	spec := ToSDKSpec(&CreateRequest{Policy: pol}, false)
	if spec.Policy == nil {
		t.Fatal("policy missing from SDK spec")
	}
	if spec.Policy.Version != 1 {
		t.Errorf("version = %d", spec.Policy.Version)
	}
	if spec.Policy.Filesystem == nil || !spec.Policy.Filesystem.IncludeWorkdir {
		t.Error("filesystem policy not propagated")
	}
}

func TestToSDKSpec_TTY(t *testing.T) {
	spec := ToSDKSpec(&CreateRequest{}, true)
	if !spec.TTY {
		t.Error("ttyResolved=true should set spec.TTY")
	}
}
