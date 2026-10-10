package cautem

import (
	"context"
	"net"
	"testing"

	controlv1 "github.com/cautem/cautem-gateway/api/gen/cautem/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type operationTestServer struct {
	controlv1.UnimplementedOperationsServiceServer
}

func (operationTestServer) GetOperation(_ context.Context, request *controlv1.GetOperationRequest) (*controlv1.GetOperationResponse, error) {
	return &controlv1.GetOperationResponse{Operation: &controlv1.OperationSummary{
		Id: "operation-1", RequestId: request.GetRequestId(), Workspace: request.GetWorkspace(), Sandbox: "demo",
		Action: "sandbox.start", State: "succeeded", ResultRegistryStatus: "running", ResultResourceVersion: 8,
	}}, nil
}

func TestGetControlOperationMapsRecoveryRecord(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	controlv1.RegisterOperationsServiceServer(server, operationTestServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := &Client{rpcConn: connection}
	operation, err := client.GetControlOperation(context.Background(), "team", "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if operation.ID != "operation-1" || operation.RequestID != "request-1" || operation.Workspace != "team" || operation.State != "succeeded" || operation.ResultResourceVersion != 8 {
		t.Fatalf("unexpected operation: %+v", operation)
	}
}

func TestNewRequestIDIsOpaqueAndUnique(t *testing.T) {
	first, err := newRequestID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newRequestID()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 || len(second) != 32 || first == second {
		t.Fatalf("invalid request IDs: %q %q", first, second)
	}
}
