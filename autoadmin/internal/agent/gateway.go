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
}
type session struct {
	agentID        string
	stream         pb.AgentChannel_SessionServer
	sendMu         sync.Mutex
	mu             sync.Mutex
	pending        map[string]chan *pb.AutomationExecuteResponse
	terminalEvents map[string]chan *pb.AgentFrame
	fileEvents     map[string]chan *pb.AgentFrame
}

func NewGateway(validate func(string, string) bool) *Gateway {
	return &Gateway{sessions: make(map[string]*session), validate: validate}
}
func (gateway *Gateway) Register(server *grpc.Server) { pb.RegisterAgentChannelServer(server, gateway) }

func (gateway *Gateway) IsOnline(agentID string) bool {
	if gateway == nil || agentID == "" {
		return false
	}
	gateway.mu.RLock()
	defer gateway.mu.RUnlock()
	return gateway.sessions[agentID] != nil
}

func (gateway *Gateway) Session(stream pb.AgentChannel_SessionServer) error {
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	hello := frame.GetHello()
	if hello == nil || hello.AgentId == "" || (gateway.validate != nil && !gateway.validate(hello.AgentId, hello.Token)) {
		agentID := ""
		if hello != nil {
			agentID = hello.AgentId
		}
		slog.Warn("agent session rejected", "agent_id", agentID)
		_ = stream.Send(&pb.ServerFrame{Payload: &pb.ServerFrame_HelloAck{HelloAck: &pb.HelloAck{Accepted: false, Message: "agent authentication failed"}}})
		return fmt.Errorf("agent authentication failed")
	}
	sess := &session{agentID: hello.AgentId, stream: stream, pending: make(map[string]chan *pb.AutomationExecuteResponse), terminalEvents: make(map[string]chan *pb.AgentFrame), fileEvents: make(map[string]chan *pb.AgentFrame)}
	gateway.mu.Lock()
	old := gateway.sessions[sess.agentID]
	gateway.sessions[sess.agentID] = sess
	sessionCount := len(gateway.sessions)
	gateway.mu.Unlock()
	slog.Info("agent session established", "agent_id", sess.agentID, "session_count", sessionCount)
	if old != nil {
		old.closePending()
	}
	defer func() {
		gateway.mu.Lock()
		if gateway.sessions[sess.agentID] == sess {
			delete(gateway.sessions, sess.agentID)
		}
		sessionCount := len(gateway.sessions)
		gateway.mu.Unlock()
		slog.Info("agent session ended", "agent_id", sess.agentID, "session_count", sessionCount)
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
func (gateway *Gateway) Execute(ctx context.Context, agentID string, request *pb.AutomationExecuteRequest) (*pb.AutomationExecuteResponse, error) {
	gateway.mu.RLock()
	sess := gateway.sessions[agentID]
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
		sess.mu.Lock()
		delete(sess.pending, request.RequestId)
		sess.mu.Unlock()
		return nil, err
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
