package agent

import (
	"context"
	"net"
	"testing"
	"time"

	"autoadmin/internal/agent/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestGatewayExecuteRoundTrip(t *testing.T) {
	const token = "test-token"
	gateway := NewGateway(func(agentID, receivedToken string) bool { return agentID == "agent-1" && receivedToken == token })
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	gateway.Register(server)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connection, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	stream, err := pb.NewAgentChannelClient(connection).Session(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_Hello{Hello: &pb.Hello{AgentId: "agent-1", Token: token}}}); err != nil {
		t.Fatal(err)
	}
	ack, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if !ack.GetHelloAck().Accepted {
		t.Fatalf("agent was rejected: %s", ack.GetHelloAck().Message)
	}

	result := make(chan *pb.AutomationExecuteRequest, 1)
	go func() {
		frame, receiveErr := stream.Recv()
		if receiveErr != nil {
			return
		}
		request := frame.GetAutomationExecuteRequest()
		result <- request
		_ = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_AutomationExecuteResponse{AutomationExecuteResponse: &pb.AutomationExecuteResponse{RequestId: request.RequestId, JobId: request.JobId, Status: "success", ExitCode: 0, Stdout: "running"}}})
	}()

	response, err := gateway.Execute(ctx, "agent-1", &pb.AutomationExecuteRequest{RequestId: "request-1", JobId: "job-1", Type: "custom", Action: "control_application", ParamsJson: `{"control_action":"status"}`})
	if err != nil {
		t.Fatal(err)
	}
	if response.JobId != "job-1" || response.Status != "success" || response.Stdout != "running" {
		t.Fatalf("unexpected response: %#v", response)
	}
	select {
	case request := <-result:
		if request.Action != "control_application" || request.RequestId != "request-1" {
			t.Fatalf("unexpected request: %#v", request)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
