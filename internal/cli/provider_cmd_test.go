package cli

import (
	"strings"
	"testing"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"go.uber.org/mock/gomock"

	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/gateway/mock"
)

func TestProviderCreate_WithMockGateway(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)

	gw.EXPECT().
		CreateProvider(gomock.Any(), "default", gomock.Any()).
		DoAndReturn(func(_ any, _ string, p *types.Provider) (*types.Provider, error) {
			if p.Name != "p1" || p.Type != "github" {
				t.Errorf("provider = %+v", p)
			}
			if p.Spec.Credentials["TOKEN"] != "abc" {
				t.Errorf("credentials = %+v", p.Spec.Credentials)
			}
			if p.Spec.Config["org"] != "acme" {
				t.Errorf("config = %+v", p.Spec.Config)
			}
			return p, nil
		})

	out, err := runCmdWithGateway(t, cliDeps{Gateway: gw},
		"provider", "create", "--name", "p1", "--type", "github",
		"--credential", "TOKEN=abc", "--config", "org=acme")
	if err != nil {
		t.Fatalf("provider create: %v", err)
	}
	if !strings.Contains(out, "p1") {
		t.Errorf("output missing provider name: %q", out)
	}
}

func TestProviderCreate_MissingNameOrType(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl) // no calls expected

	_, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "provider", "create", "--name", "p1")
	if exitCodeFor(err) != ExitUsage {
		t.Fatalf("err = %v, want a usage error", err)
	}
}

func TestProviderCreate_NoCredentialSourceIsUsageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl) // no calls expected

	_, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "provider", "create", "--name", "p1", "--type", "github")
	if exitCodeFor(err) != ExitUsage {
		t.Fatalf("err = %v, want a usage error", err)
	}
}

func TestProviderCreate_FromExistingNotYetSupported(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl) // no calls expected

	_, err := runCmdWithGateway(t, cliDeps{Gateway: gw},
		"provider", "create", "--name", "p1", "--type", "github", "--from-existing")
	if exitCodeFor(err) != ExitUsage {
		t.Fatalf("err = %v, want a usage error", err)
	}
	if !strings.Contains(err.Error(), "from-existing") {
		t.Errorf("error should name the unsupported flag: %v", err)
	}
}

func TestProviderGet_WithMockGateway(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)

	gw.EXPECT().
		GetProvider(gomock.Any(), "default", "p1").
		Return(&types.Provider{Name: "p1", Type: "github"}, nil)

	out, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "provider", "get", "p1")
	if err != nil {
		t.Fatalf("provider get: %v", err)
	}
	if !strings.Contains(out, "p1") || !strings.Contains(out, "github") {
		t.Errorf("output missing fields: %q", out)
	}
}

func TestProviderGet_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().
		GetProvider(gomock.Any(), "default", "ghost").
		Return(nil, &gateway.NotFoundError{Resource: "provider", Name: "ghost"})

	_, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "provider", "get", "ghost")
	if exitCodeFor(err) != ExitNotFound {
		t.Fatalf("err = %v, want not-found", err)
	}
}

func TestProviderList_WithMockGateway(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().
		ListProviders(gomock.Any(), "default", gomock.Any()).
		Return([]*types.Provider{{Name: "p1", Type: "github"}}, nil)

	out, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "provider", "list")
	if err != nil {
		t.Fatalf("provider list: %v", err)
	}
	if !strings.Contains(out, "p1") {
		t.Errorf("output missing provider: %q", out)
	}
}

func TestProviderList_NamesFlag(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().
		ListProviders(gomock.Any(), "default", gomock.Any()).
		Return([]*types.Provider{{Name: "p1"}, {Name: "p2"}}, nil)

	out, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "provider", "list", "--names")
	if err != nil {
		t.Fatalf("provider list --names: %v", err)
	}
	if out != "p1\np2\n" {
		t.Errorf("names output = %q", out)
	}
}

func TestProviderUpdate_WithMockGateway(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)

	existing := &types.Provider{
		Name:            "p1",
		Type:            "github",
		ResourceVersion: 7,
		Spec: types.ProviderSpec{
			Credentials: map[string]string{"TOKEN": "old"},
			Config:      map[string]string{"org": "acme"},
		},
	}
	gw.EXPECT().GetProvider(gomock.Any(), "default", "p1").Return(existing, nil)
	gw.EXPECT().
		UpdateProvider(gomock.Any(), "default", gomock.Any()).
		DoAndReturn(func(_ any, _ string, p *types.Provider) (*types.Provider, error) {
			if p.ResourceVersion != 7 {
				t.Errorf("resource version = %d, want 7 (preserved from Get)", p.ResourceVersion)
			}
			if p.Spec.Credentials["TOKEN"] != "new" {
				t.Errorf("credentials not updated: %+v", p.Spec.Credentials)
			}
			if p.Spec.Config["org"] != "acme" {
				t.Errorf("unrelated config should be preserved: %+v", p.Spec.Config)
			}
			return p, nil
		})

	out, err := runCmdWithGateway(t, cliDeps{Gateway: gw},
		"provider", "update", "p1", "--credential", "TOKEN=new")
	if err != nil {
		t.Fatalf("provider update: %v", err)
	}
	if !strings.Contains(out, "p1") {
		t.Errorf("output missing provider name: %q", out)
	}
}

func TestProviderUpdate_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().
		GetProvider(gomock.Any(), "default", "ghost").
		Return(nil, &gateway.NotFoundError{Resource: "provider", Name: "ghost"})

	_, err := runCmdWithGateway(t, cliDeps{Gateway: gw},
		"provider", "update", "ghost", "--credential", "TOKEN=new")
	if exitCodeFor(err) != ExitNotFound {
		t.Fatalf("err = %v, want not-found", err)
	}
}

func TestProviderDelete_WithMockGateway(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().DeleteProvider(gomock.Any(), "default", "p1").Return(nil)
	gw.EXPECT().DeleteProvider(gomock.Any(), "default", "p2").Return(nil)

	out, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "provider", "delete", "p1", "p2")
	if err != nil {
		t.Fatalf("provider delete: %v", err)
	}
	if !strings.Contains(out, "p1") || !strings.Contains(out, "p2") {
		t.Errorf("output missing deleted names: %q", out)
	}
}

func TestProviderDelete_StopsOnFirstError(t *testing.T) {
	ctrl := gomock.NewController(t)
	gw := mock.NewMockGateway(ctrl)
	gw.EXPECT().DeleteProvider(gomock.Any(), "default", "p1").Return(nil)
	gw.EXPECT().DeleteProvider(gomock.Any(), "default", "ghost").
		Return(&gateway.NotFoundError{Resource: "provider", Name: "ghost"})

	_, err := runCmdWithGateway(t, cliDeps{Gateway: gw}, "provider", "delete", "p1", "ghost")
	if exitCodeFor(err) != ExitNotFound {
		t.Fatalf("err = %v, want not-found", err)
	}
}

func TestProviderStubs_ReturnNotYetImplemented(t *testing.T) {
	cases := [][]string{
		{"provider", "list-profiles"},
		{"provider", "profile", "export", "foo"},
		{"provider", "refresh", "status", "foo"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := runCmd(t, args...)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "not yet implemented") {
				t.Errorf("error = %v, want mention of 'not yet implemented'", err)
			}
			if !strings.Contains(err.Error(), "openshell "+strings.Join(args, " ")) {
				t.Errorf("error = %v, want the equivalent upstream command named", err)
			}
		})
	}
}

func TestSandboxProviderStillRegisteredAfterRename(t *testing.T) {
	// Guards against the sandbox-scoped `sandbox provider` tree colliding with
	// or being shadowed by the new top-level `provider` command.
	if _, err := runCmd(t, "sandbox", "provider", "--help"); err != nil {
		t.Fatalf("sandbox provider --help: %v", err)
	}
	if _, err := runCmd(t, "provider", "--help"); err != nil {
		t.Fatalf("provider --help: %v", err)
	}
}
