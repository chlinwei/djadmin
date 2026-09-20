package logcollect

import (
	"context"
	"net"
	"testing"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/agent/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// 实例抽样认证走通配路径（旧版 agent 也能用）：后端 ListFiles 展开后取 mtime 最新的文件，
// 再从它尾部读取。这里用 bufconn 扮演 agent，验证"选中的是最新那个 + 只读尾部窗口"。
func TestTailRemoteFilePicksNewestGlobMatch(t *testing.T) {
	const token = "test-token"
	gateway := agent.NewGateway(func(instanceName, receivedToken string) bool {
		return instanceName == "host-01" && receivedToken == token
	}, nil)
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	gateway.Register(server)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer connection.Close()
	stream, err := pb.NewAgentChannelClient(connection).Session(ctx)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	if err = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_Hello{Hello: &pb.Hello{InstanceName: "host-01", Token: token}}}); err != nil {
		t.Fatalf("send hello: %v", err)
	}
	if _, err = stream.Recv(); err != nil {
		t.Fatalf("recv hello ack: %v", err)
	}

	const newestSize = logFormatSampleWindow + 4096
	tailData := []byte("half-line-cut-by-window\n2026-09-19 10:00:00 ERROR newest\n")
	dirs := map[string][]*pb.FileEntry{
		"/var/log": {
			{Name: "app-old", IsDir: true},
			{Name: "app-new", IsDir: true},
		},
		"/var/log/app-old": {
			{Name: "error.log", IsDir: false, Size: 5, Mtime: 1},
		},
		"/var/log/app-new": {
			{Name: "error.log", IsDir: false, Size: newestSize, Mtime: 2},
		},
	}
	go func() {
		for {
			frame, receiveErr := stream.Recv()
			if receiveErr != nil {
				return
			}
			if listRequest := frame.GetListRequest(); listRequest != nil {
				_ = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_ListResponse{ListResponse: &pb.ListResponse{
					RequestId: listRequest.RequestId, CurrentPath: listRequest.Path, Entries: dirs[listRequest.Path],
				}}})
				continue
			}
			if readRequest := frame.GetReadRequest(); readRequest != nil {
				if readRequest.Path != "/var/log/app-new/error.log" {
					t.Errorf("应读最新的文件，实际 path = %q", readRequest.Path)
				}
				if readRequest.Offset != newestSize-logFormatSampleWindow {
					t.Errorf("ReadRequest.Offset = %d, want %d（只读尾部窗口）", readRequest.Offset, newestSize-logFormatSampleWindow)
				}
				_ = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_ReadChunk{ReadChunk: &pb.ReadChunk{
					RequestId: readRequest.RequestId, Data: tailData, Eof: true, FileSize: newestSize,
				}}})
			}
		}
	}()

	handler := &Handler{gateway: gateway}
	text, err := handler.tailRemoteFile(ctx, "host-01", "/var/log/*/error.log")
	if err != nil {
		t.Fatalf("tail remote file: %v", err)
	}
	if text != "2026-09-19 10:00:00 ERROR newest\n" {
		t.Fatalf("内容 = %q, want 最新文件尾部（残行已丢）", text)
	}
}

// 一个都不匹配时给出可读错误，而不是拿原始 stat 报错糊弄。
func TestResolveRemoteGlobNoMatch(t *testing.T) {
	const token = "test-token"
	gateway := agent.NewGateway(func(instanceName, receivedToken string) bool {
		return instanceName == "host-01" && receivedToken == token
	}, nil)
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	gateway.Register(server)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer connection.Close()
	stream, err := pb.NewAgentChannelClient(connection).Session(ctx)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	if err = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_Hello{Hello: &pb.Hello{InstanceName: "host-01", Token: token}}}); err != nil {
		t.Fatalf("send hello: %v", err)
	}
	if _, err = stream.Recv(); err != nil {
		t.Fatalf("recv hello ack: %v", err)
	}
	go func() {
		for {
			frame, receiveErr := stream.Recv()
			if receiveErr != nil {
				return
			}
			if listRequest := frame.GetListRequest(); listRequest != nil {
				_ = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_ListResponse{ListResponse: &pb.ListResponse{
					RequestId: listRequest.RequestId, CurrentPath: listRequest.Path, Entries: nil,
				}}})
			}
		}
	}()

	handler := &Handler{gateway: gateway}
	if _, err = handler.resolveRemoteGlob(ctx, "host-01", "/var/log/*/error.log"); err == nil {
		t.Fatal("没有任何匹配时必须报错")
	}
}
