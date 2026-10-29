//go:build linux

package grpcfile

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/grpcfile/pb"
	"google.golang.org/grpc"
)

type tailProtocolServer struct {
	pb.UnimplementedAgentChannelServer
	path   string
	result chan error
}

func (server *tailProtocolServer) Session(stream pb.AgentChannel_SessionServer) error {
	err := server.exercise(stream)
	server.result <- err
	return err
}

func (server *tailProtocolServer) exercise(stream pb.AgentChannel_SessionServer) error {
	hello, err := stream.Recv()
	if err != nil || hello.GetHello().InstanceName != "tail-test-host" {
		return fmt.Errorf("hello failed: %v", err)
	}
	if err = stream.Send(&pb.ServerFrame{Payload: &pb.ServerFrame_HelloAck{HelloAck: &pb.HelloAck{Accepted: true}}}); err != nil {
		return err
	}
	send := func(action, identity string) error {
		return stream.Send(&pb.ServerFrame{Payload: &pb.ServerFrame_TailRequest{TailRequest: &pb.TailRequest{
			RequestId: "tail-1", Action: action, Path: server.path, Identity: identity,
		}}})
	}
	if err = send("prepare", ""); err != nil {
		return err
	}
	frame, err := stream.Recv()
	if err != nil || frame.GetTailEvent().Type != "ready" || frame.GetTailEvent().Identity == "" {
		return fmt.Errorf("ready failed: %v", err)
	}
	if err = send("start", frame.GetTailEvent().Identity); err != nil {
		return err
	}
	// Exercise an ordinary file command while native tail is active. This
	// validates dispatch/multiplexing, not a production throughput benchmark.
	if err = stream.Send(&pb.ServerFrame{Payload: &pb.ServerFrame_StatRequest{StatRequest: &pb.StatRequest{RequestId: "stat-1", Path: server.path}}}); err != nil {
		return err
	}
	sawStat, sawInitial := false, false
	for !sawStat || !sawInitial {
		frame, err = stream.Recv()
		if err != nil {
			return err
		}
		if stat := frame.GetStatResponse(); stat != nil {
			if stat.RequestId != "stat-1" || stat.Error != "" {
				return fmt.Errorf("stat failed: %+v", stat)
			}
			sawStat = true
		}
		if event := frame.GetTailEvent(); event != nil && event.Type == "data" {
			sawInitial = strings.Contains(string(event.Data), "initial")
		}
	}
	file, err := os.OpenFile(server.path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = file.WriteString("incremental\n")
	file.Close()
	if err != nil {
		return err
	}
	for {
		frame, err = stream.Recv()
		if err != nil {
			return err
		}
		if event := frame.GetTailEvent(); event != nil && event.Type == "data" && strings.Contains(string(event.Data), "incremental") {
			break
		}
	}
	if err = send("renew", ""); err != nil {
		return err
	}
	if err = send("stop", ""); err != nil {
		return err
	}
	for {
		frame, err = stream.Recv()
		if err != nil {
			return err
		}
		if event := frame.GetTailEvent(); event != nil && (event.Type == "end" || event.Type == "error") {
			return nil
		}
	}
}

func TestTailNativeGRPCSession(t *testing.T) {
	dir := tailTestDir(t)
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("initial\n"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	peer := &tailProtocolServer{path: path, result: make(chan error, 1)}
	pb.RegisterAgentChannelServer(server, peer)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	clientDone := make(chan error, 1)
	go func() {
		_, err := runOnce(ctx, listener.Addr().String(), "tail-test-host", "secret", nil, func(bool) {})
		clientDone <- err
	}()
	select {
	case err = <-peer.result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("native gRPC tail/ordinary command/stop round trip timed out")
	}
	cancel()
	select {
	case <-clientDone:
	case <-time.After(time.Second):
		t.Fatal("agent session did not finish")
	}
}
