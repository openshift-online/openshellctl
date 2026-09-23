package sandbox

import (
	"context"
	"errors"
	"strings"
	"testing"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	dm "github.com/NVIDIA/OpenShell/sdk/go/proto/datamodelv1"
	pb "github.com/NVIDIA/OpenShell/sdk/go/proto/openshellv1"
	"go.uber.org/mock/gomock"

	"github.com/openshift-online/openshellctl/pkg/api/v1alpha1"
	"github.com/openshift-online/openshellctl/pkg/gateway/mock"
)

func TestCreate_NoProviders(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().CreateSandbox(gomock.Any(), "default", "sb", gomock.Any(), gomock.Any()).
		Return(&types.Sandbox{ID: "id-1", Name: "sb"}, nil)

	req := &CreateRequest{Workspace: "default", Name: "sb", Image: "img"}
	res, err := Create(context.Background(), CreateDeps{GW: gw}, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Sandbox.Name != "sb" {
		t.Errorf("sandbox = %+v", res.Sandbox)
	}
}

func TestCreate_ProviderResolvesExisting(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().ListProviders(gomock.Any(), "default", gomock.Any()).
		Return([]*types.Provider{{Name: "my-openai", Type: "openai"}}, nil)
	gw.EXPECT().CreateSandbox(gomock.Any(), "default", "sb", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, spec *types.SandboxSpec, _ map[string]string) (*types.Sandbox, error) {
			if len(spec.Providers) != 1 || spec.Providers[0] != "my-openai" {
				t.Errorf("spec.Providers = %v, want [my-openai]", spec.Providers)
			}
			return &types.Sandbox{Name: "sb"}, nil
		})

	req := &CreateRequest{Workspace: "default", Name: "sb", Providers: []string{"my-openai"}}
	if _, err := Create(context.Background(), CreateDeps{GW: gw}, req, false); err != nil {
		t.Fatal(err)
	}
}

func TestCreate_MissingProviderTypeErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().ListProviders(gomock.Any(), "default", gomock.Any()).Return(nil, nil)

	req := &CreateRequest{Workspace: "default", Name: "sb", Providers: []string{"anthropic"}}
	_, err := Create(context.Background(), CreateDeps{GW: gw}, req, false)
	var e *ErrAutoProviderUnsupported
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want ErrAutoProviderUnsupported", err)
	}
}

func TestCreate_MissingProviderSkippedWithNoAutoProviders(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().ListProviders(gomock.Any(), "default", gomock.Any()).Return(nil, nil)
	gw.EXPECT().CreateSandbox(gomock.Any(), "default", "sb", gomock.Any(), gomock.Any()).
		Return(&types.Sandbox{Name: "sb"}, nil)

	no := false
	req := &CreateRequest{Workspace: "default", Name: "sb", Providers: []string{"anthropic"}, AutoProviders: &no}
	if _, err := Create(context.Background(), CreateDeps{GW: gw}, req, false); err != nil {
		t.Fatalf("skip path should succeed: %v", err)
	}
}

func TestErrorMessages_Sandbox(t *testing.T) {
	if got := (&ErrLocalBuildUnsupported{}).Error(); !strings.Contains(got, "local image builds are not supported") {
		t.Errorf("ErrLocalBuildUnsupported = %q", got)
	}
	if got := (&ErrNoDockerfile{}).Error(); got != "No Dockerfile found" {
		t.Errorf("ErrNoDockerfile = %q", got)
	}
	if got := (&ErrLocalPathMissing{Path: "/x"}).Error(); !strings.Contains(got, "/x") {
		t.Errorf("ErrLocalPathMissing = %q", got)
	}
	if got := (&ErrProviderNotFound{Name: "z"}).Error(); !strings.Contains(got, "z") {
		t.Errorf("ErrProviderNotFound = %q", got)
	}
	if got := (&ErrAutoProviderUnsupported{Type: "anthropic"}).Error(); !strings.Contains(got, "openshellctl cannot auto-create") {
		t.Errorf("ErrAutoProviderUnsupported = %q", got)
	}
	if got := SkipProviderMessage("anthropic"); !strings.Contains(got, "Skipping provider 'anthropic'") {
		t.Errorf("SkipProviderMessage = %q", got)
	}
	if got := (&ErrLifecycle{Target: "Ready"}).Error(); !strings.Contains(got, "Ready") {
		t.Errorf("ErrLifecycle = %q", got)
	}
	if got := (&ErrDeleteTimeout{Name: "sb", After: 0}).Error(); !strings.Contains(got, "sb") {
		t.Errorf("ErrDeleteTimeout = %q", got)
	}
}

func TestCoerceJSON(t *testing.T) {
	in := map[string]any{"driver": map[string]any{"count": 5}}
	out := coerceJSON(in)
	// After JSON round-trip, integers become float64.
	inner := out["driver"].(map[string]any)
	if _, ok := inner["count"].(float64); !ok {
		t.Errorf("count not coerced to float64: %T", inner["count"])
	}
}

func TestToSDKSpec_DriverConfigCoerced(t *testing.T) {
	req := &CreateRequest{DriverConfig: map[string]any{"d": map[string]any{"n": 1}}}
	spec := ToSDKSpec(req, false)
	if spec.Template == nil || spec.Template.DriverConfig == nil {
		t.Fatal("driver config should populate template")
	}
}

func TestCreate_ProviderInferenceFromCommand(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	// providers_v2 disabled → inference kept; claude command → claude-code type.
	gw.EXPECT().GetGatewayConfig(gomock.Any()).Return(&types.GatewayConfig{
		Settings: map[string]types.SettingValue{"providers_v2_enabled": {Type: types.SettingValueBool, BoolVal: false}},
	}, nil)
	gw.EXPECT().ListProviders(gomock.Any(), "default", gomock.Any()).
		Return([]*types.Provider{{Name: "cc", Type: "claude-code"}}, nil)
	gw.EXPECT().CreateSandbox(gomock.Any(), "default", "sb", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, spec *types.SandboxSpec, _ map[string]string) (*types.Sandbox, error) {
			if len(spec.Providers) != 1 || spec.Providers[0] != "cc" {
				t.Errorf("inferred provider not attached: %v", spec.Providers)
			}
			return &types.Sandbox{Name: "sb"}, nil
		})

	req := &CreateRequest{Workspace: "default", Name: "sb", Command: []string{"/usr/bin/claude"}}
	if _, err := Create(context.Background(), CreateDeps{GW: gw}, req, false); err != nil {
		t.Fatal(err)
	}
}

func TestCreate_ProviderInferenceDroppedWhenV2Enabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().GetGatewayConfig(gomock.Any()).Return(&types.GatewayConfig{
		Settings: map[string]types.SettingValue{"providers_v2_enabled": {Type: types.SettingValueBool, BoolVal: true}},
	}, nil)
	// No ListProviders expected (inference dropped, no explicit providers).
	gw.EXPECT().CreateSandbox(gomock.Any(), "default", "sb", gomock.Any(), gomock.Any()).
		Return(&types.Sandbox{Name: "sb"}, nil)

	req := &CreateRequest{Workspace: "default", Name: "sb", Command: []string{"/usr/bin/claude"}}
	if _, err := Create(context.Background(), CreateDeps{GW: gw}, req, false); err != nil {
		t.Fatal(err)
	}
}

func TestCreate_CredentialWarnings(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().CreateSandbox(gomock.Any(), "default", "sb", gomock.Any(), gomock.Any()).
		Return(&types.Sandbox{Name: "sb"}, nil)

	var stderr strings.Builder
	req := &CreateRequest{Workspace: "default", Name: "sb", Env: map[string]string{"MY_SECRET_TOKEN": "x"}}
	if _, err := Create(context.Background(), CreateDeps{GW: gw, Stderr: &stderr}, req, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "MY_SECRET_TOKEN looks like a credential") {
		t.Errorf("expected credential warning, got: %q", stderr.String())
	}
}

func TestCreate_BareGPURawPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().CreateSandboxRaw(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *pb.CreateSandboxRequest) (*pb.Sandbox, error) {
			if req.Spec.ResourceRequirements == nil || req.Spec.ResourceRequirements.Gpu == nil {
				t.Errorf("raw request missing GPU resource requirement: %+v", req.Spec)
			}
			if req.Spec.ResourceRequirements.Gpu.Count != nil {
				t.Error("bare --gpu should send Count=nil")
			}
			return &pb.Sandbox{Metadata: &dm.ObjectMeta{Name: "sb"}, Status: &pb.SandboxStatus{Phase: pb.SandboxPhase_SANDBOX_PHASE_PROVISIONING}}, nil
		})

	req := &CreateRequest{Workspace: "default", Name: "sb", GPU: &v1alpha1.GPU{}}
	res, err := Create(context.Background(), CreateDeps{GW: gw}, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Sandbox.Name != "sb" {
		t.Errorf("sandbox = %+v", res.Sandbox)
	}
}

func TestCreate_BareGPUWithPolicyRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	req := &CreateRequest{
		Workspace: "default", Name: "sb",
		GPU:    &v1alpha1.GPU{},
		Policy: &types.SandboxPolicy{Version: 1},
	}
	_, err := Create(context.Background(), CreateDeps{GW: mock.NewMockGateway(ctrl)}, req, false)
	if !errors.Is(err, ErrRawGPUWithPolicy) {
		t.Fatalf("err = %v, want ErrRawGPUWithPolicy", err)
	}
}

func TestCreate_PolicyFlowsToGateway(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	pol := &types.SandboxPolicy{
		Version:    1,
		Filesystem: &types.FilesystemPolicy{IncludeWorkdir: true, ReadOnly: []string{"/etc"}},
	}
	gw.EXPECT().CreateSandbox(gomock.Any(), "default", "sb", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, spec *types.SandboxSpec, _ map[string]string) (*types.Sandbox, error) {
			if spec.Policy == nil {
				t.Fatal("spec.Policy is nil — policy was not propagated to gateway")
			}
			if spec.Policy.Version != 1 {
				t.Errorf("spec.Policy.Version = %d, want 1", spec.Policy.Version)
			}
			if spec.Policy.Filesystem == nil || !spec.Policy.Filesystem.IncludeWorkdir {
				t.Error("spec.Policy.Filesystem not propagated")
			}
			if len(spec.Policy.Filesystem.ReadOnly) != 1 || spec.Policy.Filesystem.ReadOnly[0] != "/etc" {
				t.Errorf("spec.Policy.Filesystem.ReadOnly = %v", spec.Policy.Filesystem.ReadOnly)
			}
			return &types.Sandbox{Name: "sb"}, nil
		})

	req := &CreateRequest{Workspace: "default", Name: "sb", Policy: pol}
	res, err := Create(context.Background(), CreateDeps{GW: gw}, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Sandbox.Name != "sb" {
		t.Errorf("sandbox = %+v", res.Sandbox)
	}
}

func TestCreate_PolicyWithExplicitGPUCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	pol := &types.SandboxPolicy{Version: 1}
	cnt := uint32(2)
	gw.EXPECT().CreateSandbox(gomock.Any(), "default", "sb", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, spec *types.SandboxSpec, _ map[string]string) (*types.Sandbox, error) {
			if spec.Policy == nil {
				t.Fatal("policy should be present with explicit GPU count")
			}
			if spec.GPUCount == nil || *spec.GPUCount != 2 {
				t.Errorf("GPUCount = %v", spec.GPUCount)
			}
			return &types.Sandbox{Name: "sb"}, nil
		})

	req := &CreateRequest{
		Workspace: "default", Name: "sb",
		Policy: pol,
		GPU:    &v1alpha1.GPU{Count: &cnt},
	}
	if _, err := Create(context.Background(), CreateDeps{GW: gw}, req, false); err != nil {
		t.Fatal(err)
	}
}

func TestMerge_GPUFromManifest(t *testing.T) {
	cnt := uint32(4)
	m := &v1alpha1.Sandbox{Spec: v1alpha1.SandboxSpec{Resources: &v1alpha1.Resources{GPU: &v1alpha1.GPU{Count: &cnt}}}}
	r, _ := MergeManifestAndFlags(m, CreateFlags{})
	if r.GPU == nil || r.GPU.Count == nil || *r.GPU.Count != 4 {
		t.Errorf("GPU not merged from manifest: %+v", r.GPU)
	}
}
