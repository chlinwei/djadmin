package agent

import (
	"context"
	"fmt"
	"io"
	"time"

	"autoadmin/internal/agent/pb"
)

func fileResponseRequestID(frame *pb.AgentFrame) string {
	switch {
	case frame.GetListResponse() != nil:
		return frame.GetListResponse().RequestId
	case frame.GetStatResponse() != nil:
		return frame.GetStatResponse().RequestId
	case frame.GetReadChunk() != nil:
		return frame.GetReadChunk().RequestId
	case frame.GetWriteOpenResponse() != nil:
		return frame.GetWriteOpenResponse().RequestId
	case frame.GetWriteChunkAck() != nil:
		return frame.GetWriteChunkAck().RequestId
	case frame.GetWriteCloseResponse() != nil:
		return frame.GetWriteCloseResponse().RequestId
	case frame.GetRenameResponse() != nil:
		return frame.GetRenameResponse().RequestId
	case frame.GetDeleteResponse() != nil:
		return frame.GetDeleteResponse().RequestId
	case frame.GetMkdirResponse() != nil:
		return frame.GetMkdirResponse().RequestId
	case frame.GetCreateFileResponse() != nil:
		return frame.GetCreateFileResponse().RequestId
	default:
		return ""
	}
}

func (gateway *Gateway) requestFile(ctx context.Context, agentID, requestID string, frame *pb.ServerFrame) (*pb.AgentFrame, error) {
	gateway.mu.RLock()
	sess := gateway.sessions[agentID]
	gateway.mu.RUnlock()
	if sess == nil {
		return nil, ErrAgentOffline
	}
	result := make(chan *pb.AgentFrame, 1)
	sess.mu.Lock()
	sess.fileEvents[requestID] = result
	sess.mu.Unlock()
	if err := sess.send(frame); err != nil {
		sess.mu.Lock()
		delete(sess.fileEvents, requestID)
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
		delete(sess.fileEvents, requestID)
		sess.mu.Unlock()
		return nil, ctx.Err()
	}
}

func fileRequestID() string { return fmt.Sprintf("file-%d", time.Now().UnixNano()) }

func (gateway *Gateway) ListFiles(ctx context.Context, agentID, path string) (*pb.ListResponse, error) {
	id := fileRequestID()
	frame, err := gateway.requestFile(ctx, agentID, id, &pb.ServerFrame{Payload: &pb.ServerFrame_ListRequest{ListRequest: &pb.ListRequest{RequestId: id, Path: path}}})
	if err != nil {
		return nil, err
	}
	return frame.GetListResponse(), nil
}

func (gateway *Gateway) RenameFile(ctx context.Context, agentID, path, newName string) (*pb.RenameResponse, error) {
	id := fileRequestID()
	frame, err := gateway.requestFile(ctx, agentID, id, &pb.ServerFrame{Payload: &pb.ServerFrame_RenameRequest{RenameRequest: &pb.RenameRequest{RequestId: id, Path: path, NewName: newName}}})
	if err != nil {
		return nil, err
	}
	return frame.GetRenameResponse(), nil
}

func (gateway *Gateway) DeleteFile(ctx context.Context, agentID, path string, recursive bool) (*pb.DeleteResponse, error) {
	id := fileRequestID()
	frame, err := gateway.requestFile(ctx, agentID, id, &pb.ServerFrame{Payload: &pb.ServerFrame_DeleteRequest{DeleteRequest: &pb.DeleteRequest{RequestId: id, Path: path, Recursive: recursive}}})
	if err != nil {
		return nil, err
	}
	return frame.GetDeleteResponse(), nil
}

func (gateway *Gateway) MakeDirectory(ctx context.Context, agentID, path, name string) (*pb.MkdirResponse, error) {
	id := fileRequestID()
	frame, err := gateway.requestFile(ctx, agentID, id, &pb.ServerFrame{Payload: &pb.ServerFrame_MkdirRequest{MkdirRequest: &pb.MkdirRequest{RequestId: id, Path: path, Name: name}}})
	if err != nil {
		return nil, err
	}
	return frame.GetMkdirResponse(), nil
}

func (gateway *Gateway) StatFile(ctx context.Context, agentID, path string) (*pb.StatResponse, error) {
	id := fileRequestID()
	frame, err := gateway.requestFile(ctx, agentID, id, &pb.ServerFrame{Payload: &pb.ServerFrame_StatRequest{StatRequest: &pb.StatRequest{RequestId: id, Path: path}}})
	if err != nil {
		return nil, err
	}
	return frame.GetStatResponse(), nil
}

func (gateway *Gateway) ReadFileChunk(ctx context.Context, agentID, path string, offset, length int64) (*pb.ReadChunk, error) {
	id := fileRequestID()
	frame, err := gateway.requestFile(ctx, agentID, id, &pb.ServerFrame{Payload: &pb.ServerFrame_ReadRequest{ReadRequest: &pb.ReadRequest{RequestId: id, Path: path, Offset: offset, Length: length}}})
	if err != nil {
		return nil, err
	}
	return frame.GetReadChunk(), nil
}

func (gateway *Gateway) WriteFile(ctx context.Context, agentID, dirPath, fileName string, reader io.Reader) (*pb.WriteCloseResponse, error) {
	id := fileRequestID()
	request := func(frame *pb.ServerFrame) (*pb.AgentFrame, error) {
		return gateway.requestFile(ctx, agentID, id, frame)
	}
	opened, err := request(&pb.ServerFrame{Payload: &pb.ServerFrame_WriteOpenRequest{WriteOpenRequest: &pb.WriteOpenRequest{RequestId: id, DirPath: dirPath, FileName: fileName}}})
	if err != nil {
		return nil, err
	}
	if result := opened.GetWriteOpenResponse(); result == nil || result.Error != "" {
		return nil, fmt.Errorf("open remote file: %s", result.GetError())
	}
	buffer := make([]byte, 1024*1024)
	for {
		count, readErr := reader.Read(buffer)
		if count > 0 {
			ack, sendErr := request(&pb.ServerFrame{Payload: &pb.ServerFrame_WriteChunkRequest{WriteChunkRequest: &pb.WriteChunkRequest{RequestId: id, Data: buffer[:count]}}})
			if sendErr != nil {
				return nil, sendErr
			}
			if result := ack.GetWriteChunkAck(); result == nil || result.Error != "" {
				return nil, fmt.Errorf("write remote file: %s", result.GetError())
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	closed, err := request(&pb.ServerFrame{Payload: &pb.ServerFrame_WriteCloseRequest{WriteCloseRequest: &pb.WriteCloseRequest{RequestId: id}}})
	if err != nil {
		return nil, err
	}
	result := closed.GetWriteCloseResponse()
	if result == nil || result.Error != "" {
		return nil, fmt.Errorf("close remote file: %s", result.GetError())
	}
	return result, nil
}
