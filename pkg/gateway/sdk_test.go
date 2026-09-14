package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
)

func newFakeGateway(opts ...fake.ClientOption) *client {
	fc := fake.NewClient(opts...)
	return &client{conn: &Conn{SDK: fc}}
}

func TestSDK_CreateGetList(t *testing.T) {
	c := newFakeGateway()
	ctx := context.Background()

	sb, err := c.CreateSandbox(ctx, "default", "sb-1", &types.SandboxSpec{}, map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if sb.Status.Phase != types.SandboxProvisioning {
		t.Errorf("phase = %v, want Provisioning", sb.Status.Phase)
	}

	got, err := c.GetSandbox(ctx, "default", "sb-1")
	if err != nil {
		t.Fatalf("GetSandbox: %v", err)
	}
	if got.Name != "sb-1" {
		t.Errorf("name = %q", got.Name)
	}

	list, err := c.ListSandboxes(ctx, "default", types.ListOptions{})
	if err != nil {
		t.Fatalf("ListSandboxes: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("list len = %d, want 1", len(list))
	}
}

func TestSDK_GetNotFoundClassified(t *testing.T) {
	c := newFakeGateway()
	_, err := c.GetSandbox(context.Background(), "default", "ghost")
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want NotFoundError", err)
	}
}

func TestSDK_CreateAlreadyExistsClassified(t *testing.T) {
	c := newFakeGateway()
	ctx := context.Background()
	if _, err := c.CreateSandbox(ctx, "default", "dup", &types.SandboxSpec{}, nil); err != nil {
		t.Fatal(err)
	}
	_, err := c.CreateSandbox(ctx, "default", "dup", &types.SandboxSpec{}, nil)
	var ae *AlreadyExistsError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v, want AlreadyExistsError", err)
	}
}

func TestSDK_StopStart(t *testing.T) {
	c := newFakeGateway()
	ctx := context.Background()
	if _, err := c.CreateSandbox(ctx, "default", "sb", &types.SandboxSpec{}, nil); err != nil {
		t.Fatal(err)
	}
	stopped, err := c.StopSandbox(ctx, "default", "sb")
	if err != nil {
		t.Fatalf("StopSandbox: %v", err)
	}
	if stopped.Status.Phase != types.SandboxStopped {
		t.Errorf("phase = %v, want Stopped", stopped.Status.Phase)
	}
	started, err := c.StartSandbox(ctx, "default", "sb")
	if err != nil {
		t.Fatalf("StartSandbox: %v", err)
	}
	if started.Status.Phase != types.SandboxReady {
		t.Errorf("phase = %v, want Ready", started.Status.Phase)
	}
}

func TestSDK_CurrentUser(t *testing.T) {
	want := &types.CurrentUser{Subject: "user-1", Roles: []string{"openshell-user"}}
	c := newFakeGateway(fake.WithCurrentUser(want))
	got, err := c.CurrentUser(context.Background())
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if got.Subject != "user-1" || len(got.Roles) != 1 {
		t.Errorf("current user = %+v", got)
	}
}

func TestSDK_ListProviders(t *testing.T) {
	c := newFakeGateway()
	got, err := c.ListProviders(context.Background(), "default", types.ListOptions{})
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("providers = %d, want 0", len(got))
	}
}

// New wires a Gateway from a Conn; assert it returns a usable value.
func TestNew_ReturnsGateway(t *testing.T) {
	fc := fake.NewClient()
	gw := New(&Conn{SDK: fc}, nil)
	if gw == nil {
		t.Fatal("New returned nil")
	}
}
