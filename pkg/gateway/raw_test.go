package gateway

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	pb "github.com/NVIDIA/OpenShell/sdk/go/proto/openshellv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// fakeServer implements just the raw methods exercised by these tests.
type fakeServer struct {
	pb.UnimplementedOpenShellServer

	deleteResp  *pb.DeleteSandboxResponse
	deleteReq   *pb.DeleteSandboxRequest
	watchEvents []*pb.SandboxStreamEvent
	execEvents  []*pb.ExecSandboxEvent
	deleteErr   error
}

func (s *fakeServer) DeleteSandbox(_ context.Context, req *pb.DeleteSandboxRequest) (*pb.DeleteSandboxResponse, error) {
	s.deleteReq = req
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	return s.deleteResp, nil
}

func (s *fakeServer) WatchSandbox(_ *pb.WatchSandboxRequest, stream grpc.ServerStreamingServer[pb.SandboxStreamEvent]) error {
	for _, e := range s.watchEvents {
		if err := stream.Send(e); err != nil {
			return err
		}
	}
	return nil
}

func (s *fakeServer) ExecSandbox(_ *pb.ExecSandboxRequest, stream grpc.ServerStreamingServer[pb.ExecSandboxEvent]) error {
	for _, e := range s.execEvents {
		if err := stream.Send(e); err != nil {
			return err
		}
	}
	return nil
}

// newTestClient stands up an in-process gRPC server over bufconn and returns a
// gateway client wired to it plus a cleanup func.
func newTestClient(t *testing.T, srv *fakeServer) (*client, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	pb.RegisterOpenShellServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	c := &client{conn: &Conn{Raw: pb.NewOpenShellClient(conn), rawConn: conn}}
	cleanup := func() {
		_ = conn.Close()
		gs.Stop()
		_ = lis.Close()
	}
	return c, cleanup
}

func TestRawDeleteSandbox_PreservesBool(t *testing.T) {
	for _, want := range []bool{true, false} {
		srv := &fakeServer{deleteResp: &pb.DeleteSandboxResponse{Deleted: want}}
		c, cleanup := newTestClient(t, srv)

		got, err := c.DeleteSandbox(context.Background(), "default", "sb-1")
		cleanup()
		if err != nil {
			t.Fatalf("DeleteSandbox: %v", err)
		}
		if got != want {
			t.Errorf("deleted = %v, want %v", got, want)
		}
		if srv.deleteReq.GetName() != "sb-1" || srv.deleteReq.GetWorkspace() != "default" {
			t.Errorf("request = %+v", srv.deleteReq)
		}
	}
}

func TestRawDeleteSandbox_ClassifiesError(t *testing.T) {
	srv := &fakeServer{deleteErr: status.Error(codes.NotFound, "nope")}
	c, cleanup := newTestClient(t, srv)
	defer cleanup()

	_, err := c.DeleteSandbox(context.Background(), "default", "ghost")
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want NotFoundError", err)
	}
}

func TestRawWatchSandbox_StreamsEvents(t *testing.T) {
	srv := &fakeServer{watchEvents: []*pb.SandboxStreamEvent{
		{Payload: &pb.SandboxStreamEvent_Sandbox{Sandbox: &pb.Sandbox{}}},
		{Payload: &pb.SandboxStreamEvent_Sandbox{Sandbox: &pb.Sandbox{}}},
	}}
	c, cleanup := newTestClient(t, srv)
	defer cleanup()

	stream, err := c.WatchSandbox(context.Background(), &pb.WatchSandboxRequest{Id: "sb-1", FollowStatus: true})
	if err != nil {
		t.Fatalf("WatchSandbox: %v", err)
	}
	defer stream.Close()

	count := 0
	for {
		ev, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		if ev.GetSandbox() == nil {
			t.Errorf("expected a sandbox payload, got %+v", ev)
		}
		count++
	}
	if count != 2 {
		t.Errorf("received %d events, want 2", count)
	}
}

func TestRawExecSandbox_StreamsExit(t *testing.T) {
	srv := &fakeServer{execEvents: []*pb.ExecSandboxEvent{
		{Payload: &pb.ExecSandboxEvent_Stdout{Stdout: &pb.ExecSandboxStdout{Data: []byte("hi")}}},
		{Payload: &pb.ExecSandboxEvent_Exit{Exit: &pb.ExecSandboxExit{ExitCode: 7}}},
	}}
	c, cleanup := newTestClient(t, srv)
	defer cleanup()

	stream, err := c.ExecSandbox(context.Background(), &pb.ExecSandboxRequest{SandboxId: "sb-1", Command: []string{"true"}})
	if err != nil {
		t.Fatalf("ExecSandbox: %v", err)
	}
	defer stream.Close()

	var exit int32 = -1
	for {
		ev, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		if e := ev.GetExit(); e != nil {
			exit = e.GetExitCode()
		}
	}
	if exit != 7 {
		t.Errorf("exit code = %d, want 7", exit)
	}
}
