package sandbox

import (
	"context"
	"errors"
	"strings"
	"testing"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
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

func TestMerge_GPUFromManifest(t *testing.T) {
	cnt := uint32(4)
	m := &v1alpha1.Sandbox{Spec: v1alpha1.SandboxSpec{Resources: &v1alpha1.Resources{GPU: &v1alpha1.GPU{Count: &cnt}}}}
	r, _ := MergeManifestAndFlags(m, CreateFlags{})
	if r.GPU == nil || r.GPU.Count == nil || *r.GPU.Count != 4 {
		t.Errorf("GPU not merged from manifest: %+v", r.GPU)
	}
}
