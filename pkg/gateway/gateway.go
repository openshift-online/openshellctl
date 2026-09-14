// Package gateway is the RPC boundary for openshellctl. It dials the gateway
// (building both the typed SDK client and a raw generated-stub client over a
// second grpc.ClientConn with identical credentials), exposes the Gateway
// interface that pkg/sandbox and the operator program against, classifies SDK
// and raw errors into a typed taxonomy, and retries once on Unauthenticated.
// It never shells out; the raw stubs are used only where the SDK is lossy or a
// stub (file transfer, watch events, exec timeout/tty/stdin, the delete bool,
// bare --gpu). See spec §5.3.
package gateway

import (
	"context"
	"io"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	pb "github.com/NVIDIA/OpenShell/sdk/go/proto/openshellv1"
)

// Stream is a minimal server-streaming receiver over a raw gRPC stream.
type Stream[T any] interface {
	Recv() (T, error)
	Close()
}

// BidiStream is the interactive-exec bidirectional stream.
type BidiStream interface {
	Send(*pb.ExecSandboxInput) error
	Recv() (*pb.ExecSandboxEvent, error)
	CloseSend() error
}

// Gateway is the interface pkg/sandbox and the operator program against. The
// typed methods are served by the SDK; the raw methods use the generated stubs
// for behaviours the SDK cannot express.
//
//go:generate go run go.uber.org/mock/mockgen -destination=mock/gateway_mock.go -package=mock . Gateway
type Gateway interface {
	// typed (SDK)
	CreateSandbox(ctx context.Context, workspace, name string, spec *types.SandboxSpec, labels map[string]string) (*types.Sandbox, error)
	GetSandbox(ctx context.Context, workspace, name string) (*types.Sandbox, error)
	ListSandboxes(ctx context.Context, workspace string, opts types.ListOptions) ([]*types.Sandbox, error)
	DeleteSandbox(ctx context.Context, workspace, name string) (deleted bool, err error)
	StopSandbox(ctx context.Context, workspace, name string) (*types.Sandbox, error)
	StartSandbox(ctx context.Context, workspace, name string) (*types.Sandbox, error)
	ListSandboxProviders(ctx context.Context, workspace, sandbox string) ([]*types.Provider, error)
	AttachProvider(ctx context.Context, workspace, sandbox, provider string, expectedRV uint64) (*types.Sandbox, bool, error)
	DetachProvider(ctx context.Context, workspace, sandbox, provider string, expectedRV uint64) (*types.Sandbox, bool, error)
	ListProviders(ctx context.Context, workspace string, opts types.ListOptions) ([]*types.Provider, error)
	GetSandboxConfig(ctx context.Context, workspace, sandbox string) (*types.SandboxConfig, error)
	GetGatewayConfig(ctx context.Context) (*types.GatewayConfig, error)
	UpdateConfig(ctx context.Context, workspace string, u *types.ConfigUpdate) (*types.ConfigUpdateResult, error)
	GetLogs(ctx context.Context, workspace, sandbox string, opts ...types.LogOption) (*types.LogResult, error)
	CurrentUser(ctx context.Context) (*types.CurrentUser, error)

	// raw (generated stubs) — things the SDK cannot express
	// CreateSandboxRaw creates via the raw stub, preserving fields the SDK spec
	// cannot express (e.g. bare --gpu = ResourceRequirements{Gpu:{Count:nil}}).
	CreateSandboxRaw(ctx context.Context, req *pb.CreateSandboxRequest) (*pb.Sandbox, error)
	WatchSandbox(ctx context.Context, req *pb.WatchSandboxRequest) (Stream[*pb.SandboxStreamEvent], error)
	ExecSandbox(ctx context.Context, req *pb.ExecSandboxRequest) (Stream[*pb.ExecSandboxEvent], error)
	ExecSandboxInteractive(ctx context.Context) (BidiStream, error)
	SSHTunnel(ctx context.Context, workspace, sandbox string) (io.ReadWriteCloser, error)
	TCPListen(ctx context.Context, workspace, sandbox string, remotePort, localPort uint32, bind string) (v1.ForwardListener, error)
}
