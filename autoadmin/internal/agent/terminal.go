package agent

import (
	"context"
	"fmt"
	"time"

	"autoadmin/internal/agent/pb"
)

func (gateway *Gateway) OpenTerminal(ctx context.Context, agentID, requestID, targetUser string, cols, rows uint32) (<-chan *pb.AgentFrame, error) {
	gateway.mu.RLock()
	sess := gateway.sessions[agentID]
	gateway.mu.RUnlock()
	if sess == nil {
		return nil, ErrAgentOffline
	}
	if requestID == "" {
		requestID = fmt.Sprintf("terminal-%d", time.Now().UnixNano())
	}
	events := make(chan *pb.AgentFrame, 32)
	sess.mu.Lock()
	sess.terminalEvents[requestID] = events
	sess.mu.Unlock()
	if err := sess.send(&pb.ServerFrame{Payload: &pb.ServerFrame_TerminalOpenRequest{TerminalOpenRequest: &pb.TerminalOpenRequest{RequestId: requestID, Cols: cols, Rows: rows, TargetUser: targetUser}}}); err != nil {
		sess.mu.Lock()
		delete(sess.terminalEvents, requestID)
		sess.mu.Unlock()
		close(events)
		return nil, err
	}
	return events, nil
}

func (gateway *Gateway) SendTerminalData(ctx context.Context, agentID, requestID string, data []byte) error {
	return gateway.sendTerminal(ctx, agentID, &pb.ServerFrame{Payload: &pb.ServerFrame_TerminalDataRequest{TerminalDataRequest: &pb.TerminalDataRequest{RequestId: requestID, Data: data}}})
}
func (gateway *Gateway) ResizeTerminal(ctx context.Context, agentID, requestID string, cols, rows uint32) error {
	return gateway.sendTerminal(ctx, agentID, &pb.ServerFrame{Payload: &pb.ServerFrame_TerminalResizeRequest{TerminalResizeRequest: &pb.TerminalResizeRequest{RequestId: requestID, Cols: cols, Rows: rows}}})
}
func (gateway *Gateway) CloseTerminal(ctx context.Context, agentID, requestID string) error {
	return gateway.sendTerminal(ctx, agentID, &pb.ServerFrame{Payload: &pb.ServerFrame_TerminalCloseRequest{TerminalCloseRequest: &pb.TerminalCloseRequest{RequestId: requestID}}})
}
func (gateway *Gateway) sendTerminal(ctx context.Context, agentID string, frame *pb.ServerFrame) error {
	gateway.mu.RLock()
	sess := gateway.sessions[agentID]
	gateway.mu.RUnlock()
	if sess == nil {
		return ErrAgentOffline
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return sess.send(frame)
	}
}
