package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"autoadmin/internal/agent/pb"

	"google.golang.org/grpc"
)

var ErrAgentOffline = errors.New("agent offline")

type Gateway struct {
	pb.UnimplementedAgentChannelServer
	mu       sync.RWMutex
	sessions map[string]*session
	validate func(string, string) bool
	// onHello 在握手校验通过后回调（instance_name + Hello.version），用于把 agent 版本
	// 与在线时间落库；回调失败只记日志，不影响会话建立。
	onHello func(instanceName, version string)
}
type session struct {
	instanceName   string
	stream         pb.AgentChannel_SessionServer
	sendMu         sync.Mutex
	mu             sync.Mutex
	pending        map[string]chan *pb.AutomationExecuteResponse
	terminalEvents map[string]chan *pb.AgentFrame
	fileEvents     map[string]chan *pb.AgentFrame
}

func NewGateway(validate func(string, string) bool, onHello func(instanceName, version string)) *Gateway {
	return &Gateway{sessions: make(map[string]*session), validate: validate, onHello: onHello}
}
func (gateway *Gateway) Register(server *grpc.Server) { pb.RegisterAgentChannelServer(server, gateway) }

// IsOnline 判断该实例名的 agent 是否有活跃 gRPC 会话；instance_name 就是会话路由 key。
func (gateway *Gateway) IsOnline(instanceName string) bool {
	if gateway == nil || instanceName == "" {
		return false
	}
	gateway.mu.RLock()
	defer gateway.mu.RUnlock()
	return gateway.sessions[instanceName] != nil
}

func (gateway *Gateway) Session(stream pb.AgentChannel_SessionServer) error {
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	hello := frame.GetHello()
	if hello == nil || hello.InstanceName == "" || (gateway.validate != nil && !gateway.validate(hello.InstanceName, hello.Token)) {
		instanceName := ""
		if hello != nil {
			instanceName = hello.InstanceName
		}
		slog.Warn("agent session rejected", "instance_name", instanceName)
		_ = stream.Send(&pb.ServerFrame{Payload: &pb.ServerFrame_HelloAck{HelloAck: &pb.HelloAck{Accepted: false, Message: "agent authentication failed"}}})
		return fmt.Errorf("agent authentication failed")
	}
	sess := &session{instanceName: hello.InstanceName, stream: stream, pending: make(map[string]chan *pb.AutomationExecuteResponse), terminalEvents: make(map[string]chan *pb.AgentFrame), fileEvents: make(map[string]chan *pb.AgentFrame)}
	if gateway.onHello != nil {
		gateway.onHello(sess.instanceName, hello.Version)
	}
	gateway.mu.Lock()
	old := gateway.sessions[sess.instanceName]
	gateway.sessions[sess.instanceName] = sess
	sessionCount := len(gateway.sessions)
	gateway.mu.Unlock()
	slog.Info("agent session established", "instance_name", sess.instanceName, "session_count", sessionCount)
	if old != nil {
		old.closePending()
	}
	defer func() {
		gateway.mu.Lock()
		if gateway.sessions[sess.instanceName] == sess {
			delete(gateway.sessions, sess.instanceName)
		}
		sessionCount := len(gateway.sessions)
		gateway.mu.Unlock()
		slog.Info("agent session ended", "instance_name", sess.instanceName, "session_count", sessionCount)
		sess.closePending()
	}()
	if err = stream.Send(&pb.ServerFrame{Payload: &pb.ServerFrame_HelloAck{HelloAck: &pb.HelloAck{Accepted: true, Message: "accepted"}}}); err != nil {
		return err
	}
	for {
		frame, recvErr := stream.Recv()
		if recvErr != nil {
			return recvErr
		}
		if response := frame.GetAutomationExecuteResponse(); response != nil {
			sess.mu.Lock()
			pending := sess.pending[response.RequestId]
			delete(sess.pending, response.RequestId)
			sess.mu.Unlock()
			if pending != nil {
				pending <- response
				close(pending)
			}
		}
		if event := frame.GetTerminalOpenResponse(); event != nil {
			sess.sendTerminalEvent(event.RequestId, frame)
		}
		if event := frame.GetTerminalDataResponse(); event != nil {
			sess.sendTerminalEvent(event.RequestId, frame)
		}
		if event := frame.GetTerminalExitResponse(); event != nil {
			sess.sendTerminalEvent(event.RequestId, frame)
		}
		if requestID := fileResponseRequestID(frame); requestID != "" {
			sess.sendFileEvent(requestID, frame)
		}
	}
}
func (sess *session) closePending() {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	for id, ch := range sess.pending {
		delete(sess.pending, id)
		close(ch)
	}
	for id, ch := range sess.terminalEvents {
		delete(sess.terminalEvents, id)
		close(ch)
	}
	for id, ch := range sess.fileEvents {
		delete(sess.fileEvents, id)
		close(ch)
	}
}

// dropSession 摘除已判死的会话：仅当 sessions 里仍是该会话（未被同 ID 的新连接顶替）时删除，
// 并关闭其全部 pending 管道让等待方立刻得到离线错误。
func (gateway *Gateway) dropSession(sess *session) {
	gateway.mu.Lock()
	if gateway.sessions[sess.instanceName] == sess {
		delete(gateway.sessions, sess.instanceName)
	}
	gateway.mu.Unlock()
	sess.closePending()
}
func (sess *session) sendTerminalEvent(requestID string, frame *pb.AgentFrame) {
	sess.mu.Lock()
	channel := sess.terminalEvents[requestID]
	sess.mu.Unlock()
	if channel != nil {
		channel <- frame
	}
}
func (sess *session) sendFileEvent(requestID string, frame *pb.AgentFrame) {
	sess.mu.Lock()
	channel := sess.fileEvents[requestID]
	delete(sess.fileEvents, requestID)
	sess.mu.Unlock()
	if channel != nil {
		channel <- frame
		close(channel)
	}
}
func (sess *session) send(frame *pb.ServerFrame) error {
	sess.sendMu.Lock()
	defer sess.sendMu.Unlock()
	return sess.stream.Send(frame)
}
func (gateway *Gateway) Execute(ctx context.Context, instanceName string, request *pb.AutomationExecuteRequest) (*pb.AutomationExecuteResponse, error) {
	gateway.mu.RLock()
	sess := gateway.sessions[instanceName]
	gateway.mu.RUnlock()
	if sess == nil {
		return nil, ErrAgentOffline
	}
	if request.RequestId == "" {
		request.RequestId = fmt.Sprintf("agent-%d", time.Now().UnixNano())
	}
	result := make(chan *pb.AutomationExecuteResponse, 1)
	sess.mu.Lock()
	sess.pending[request.RequestId] = result
	sess.mu.Unlock()
	if err := sess.send(&pb.ServerFrame{Payload: &pb.ServerFrame_AutomationExecuteRequest{AutomationExecuteRequest: request}}); err != nil {
		// send 失败说明底层传输已断（典型报错 transport is closing）。此时必须把死会话
		// 从 sessions 摘除并唤醒所有等待者，否则 IsOnline 仍报在线、后续请求继续撞同一
		// 个晦涩的 gRPC 错误，而 agent 重连前 UI 无法得到"离线"语义。
		gateway.dropSession(sess)
		return nil, ErrAgentOffline
	}
	select {
	case response, ok := <-result:
		if !ok {
			return nil, ErrAgentOffline
		}
		return response, nil
	case <-ctx.Done():
		sess.mu.Lock()
		delete(sess.pending, request.RequestId)
		sess.mu.Unlock()
		return nil, ctx.Err()
	}
}
