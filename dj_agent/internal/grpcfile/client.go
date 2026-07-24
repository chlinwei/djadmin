// Package grpcfile 实现 dj-agent 侧的文件传输 gRPC 客户端：主动拨号连接 backend，
// 建立一条长连接双向流，在同一条连接上并发处理 backend 下发的 list/stat/read/write/
// rename/delete 命令。网络方向与现有 RabbitMQ 心跳/任务模型一致（agent 拨出），
// 不要求目标主机对 backend 网络可达。
package grpcfile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/chlinwei/djadmin/dj_agent/internal/executor"
	"github.com/chlinwei/djadmin/dj_agent/internal/grpcfile/pb"
)

// 默认 gRPC 消息限制是 4MB。为避免数据帧 + protobuf 开销触顶导致大文件流中断，
// 读分片默认降为 1MB；并同时在连接参数里放宽消息上限。
const readChunkSize = 1 * 1024 * 1024
const grpcMaxMessageSize = 32 * 1024 * 1024

type writeHandle struct {
	file      *os.File
	tempPath  string
	finalPath string
}

type session struct {
	stream pb.AgentChannel_SessionClient
	sendMu sync.Mutex

	// exec 复用 agent 内统一的任务执行器，让 gRPC 通道下发的自动化任务与
	// RabbitMQ 路径共享同一套执行/降权/超时语义。
	exec *executor.Executor
	// 运行状态提供器：用于通过 gRPC 返回 agent runtime 状态，替代旧 HTTP 状态接口。
	runtimeStatusProvider func() map[string]any

	writesMu sync.Mutex
	writes   map[string]*writeHandle

	terminalsMu sync.Mutex
	terminals   map[string]*terminalSession
}

func (s *session) send(frame *pb.AgentFrame) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.stream.Send(frame)
}

// Run 阻塞运行文件传输会话，断线后按指数退避自动重连，直至 ctx 被取消。
func Run(ctx context.Context, addr, agentID string, exec *executor.Executor, runtimeStatusProvider func() map[string]any) {
	if strings.TrimSpace(addr) == "" {
		slog.Warn("grpc file-transfer disabled: empty addr")
		return
	}
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := runOnce(ctx, addr, agentID, exec, runtimeStatusProvider); err != nil {
			slog.Warn("grpc file-transfer session ended", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func runOnce(ctx context.Context, addr, agentID string, exec *executor.Executor, runtimeStatusProvider func() map[string]any) error {
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(grpcMaxMessageSize),
			grpc.MaxCallSendMsgSize(grpcMaxMessageSize),
		),
	)
	if err != nil {
		return fmt.Errorf("dial backend grpc failed: %w", err)
	}
	defer conn.Close()

	client := pb.NewAgentChannelClient(conn)
	stream, err := client.Session(ctx)
	if err != nil {
		return fmt.Errorf("open session failed: %w", err)
	}

	sess := &session{
		stream:                stream,
		exec:                  exec,
		runtimeStatusProvider: runtimeStatusProvider,
		writes:                make(map[string]*writeHandle),
		terminals:             make(map[string]*terminalSession),
	}
	defer sess.closeAllTerminals()
	if err := sess.send(&pb.AgentFrame{Payload: &pb.AgentFrame_Hello{Hello: &pb.Hello{AgentId: agentID}}}); err != nil {
		return fmt.Errorf("send hello failed: %w", err)
	}

	for {
		frame, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("recv failed: %w", err)
		}
		switch payload := frame.Payload.(type) {
		case *pb.ServerFrame_HelloAck:
			if !payload.HelloAck.Accepted {
				return fmt.Errorf("hello rejected: %s", payload.HelloAck.Message)
			}
			slog.Info("grpc file-transfer session established", "agent_id", agentID, "addr", addr)
		case *pb.ServerFrame_ListRequest:
			sess.handleList(payload.ListRequest)
		case *pb.ServerFrame_StatRequest:
			sess.handleStat(payload.StatRequest)
		case *pb.ServerFrame_ReadRequest:
			// 下载可能耗时较长，放到独立 goroutine 里跑，避免阻塞同一连接上的其他并发操作。
			go sess.handleRead(payload.ReadRequest)
		case *pb.ServerFrame_WriteOpenRequest:
			sess.handleWriteOpen(payload.WriteOpenRequest)
		case *pb.ServerFrame_WriteChunkRequest:
			sess.handleWriteChunk(payload.WriteChunkRequest)
		case *pb.ServerFrame_WriteCloseRequest:
			sess.handleWriteClose(payload.WriteCloseRequest)
		case *pb.ServerFrame_RenameRequest:
			sess.handleRename(payload.RenameRequest)
		case *pb.ServerFrame_DeleteRequest:
			sess.handleDelete(payload.DeleteRequest)
		case *pb.ServerFrame_MkdirRequest:
			sess.handleMkdir(payload.MkdirRequest)
		case *pb.ServerFrame_CreateFileRequest:
			sess.handleCreateFile(payload.CreateFileRequest)
		case *pb.ServerFrame_TerminalOpenRequest:
			sess.handleTerminalOpen(payload.TerminalOpenRequest)
		case *pb.ServerFrame_TerminalDataRequest:
			sess.handleTerminalData(payload.TerminalDataRequest)
		case *pb.ServerFrame_TerminalResizeRequest:
			sess.handleTerminalResize(payload.TerminalResizeRequest)
		case *pb.ServerFrame_TerminalCloseRequest:
			sess.handleTerminalClose(payload.TerminalCloseRequest)
		case *pb.ServerFrame_AutomationExecuteRequest:
			// 自动化任务可能长时间执行，放到独立 goroutine，避免阻塞同连接的其他并发操作。
			go sess.handleAutomationExecute(ctx, payload.AutomationExecuteRequest)
		default:
			slog.Warn("grpc file-transfer received unknown frame type")
		}
	}
}

// normalizePath 把相对路径（典型如 "."）解析到 agent 进程运行用户的 home 目录下，
// 与直连 SSH 场景下 sftp_client.normalize() 的行为保持一致；绝对路径原样 Clean 返回。
func normalizePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(home, path)), nil
}

func (s *session) handleList(req *pb.ListRequest) {
	resp := &pb.ListResponse{RequestId: req.RequestId}
	absPath, err := normalizePath(req.Path)
	if err != nil {
		resp.Error = err.Error()
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_ListResponse{ListResponse: resp}})
		return
	}
	resp.CurrentPath = absPath
	items, err := os.ReadDir(absPath)
	if err != nil {
		resp.Error = err.Error()
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_ListResponse{ListResponse: resp}})
		return
	}
	entries := make([]*pb.FileEntry, 0, len(items))
	for _, item := range items {
		info, infoErr := item.Info()
		if infoErr != nil {
			continue
		}
		entries = append(entries, &pb.FileEntry{
			Name:  item.Name(),
			Size:  info.Size(),
			Mode:  uint32(info.Mode()),
			IsDir: item.IsDir(),
			Mtime: info.ModTime().Unix(),
		})
	}
	resp.Entries = entries
	_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_ListResponse{ListResponse: resp}})
}

func (s *session) handleStat(req *pb.StatRequest) {
	resp := &pb.StatResponse{RequestId: req.RequestId}
	absPath, err := normalizePath(req.Path)
	if err != nil {
		resp.Error = err.Error()
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_StatResponse{StatResponse: resp}})
		return
	}
	info, err := os.Stat(absPath)
	if err != nil {
		resp.Error = err.Error()
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_StatResponse{StatResponse: resp}})
		return
	}
	resp.NormalizedPath = absPath
	resp.Size = info.Size()
	resp.Mode = uint32(info.Mode())
	resp.IsDir = info.IsDir()
	resp.Mtime = info.ModTime().Unix()
	_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_StatResponse{StatResponse: resp}})
}

func (s *session) sendReadError(requestID string, err error) {
	_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_ReadChunk{ReadChunk: &pb.ReadChunk{
		RequestId: requestID,
		Error:     err.Error(),
		Eof:       true,
	}}})
}

func (s *session) handleRead(req *pb.ReadRequest) {
	// path 由 backend 先调用 Stat 拿到 normalized_path 后再传入，这里视为已是绝对路径；
	// 仍走 normalizePath 兜底一次（对绝对路径是幂等的 Clean 操作）。
	absPath, err := normalizePath(req.Path)
	if err != nil {
		s.sendReadError(req.RequestId, err)
		return
	}
	f, err := os.Open(absPath)
	if err != nil {
		s.sendReadError(req.RequestId, err)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		s.sendReadError(req.RequestId, err)
		return
	}
	fileSize := info.Size()

	if req.Offset > 0 {
		if _, err := f.Seek(req.Offset, io.SeekStart); err != nil {
			s.sendReadError(req.RequestId, err)
			return
		}
	}

	remaining := req.Length
	useLimit := remaining > 0
	buf := make([]byte, readChunkSize)
	for {
		toRead := len(buf)
		if useLimit {
			if remaining <= 0 {
				_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_ReadChunk{ReadChunk: &pb.ReadChunk{
					RequestId: req.RequestId, Eof: true, FileSize: fileSize,
				}}})
				return
			}
			if int64(toRead) > remaining {
				toRead = int(remaining)
			}
		}
		n, readErr := f.Read(buf[:toRead])
		if n > 0 {
			if useLimit {
				remaining -= int64(n)
			}
			isLast := errors.Is(readErr, io.EOF) || (useLimit && remaining <= 0)
			data := make([]byte, n)
			copy(data, buf[:n])
			sendErr := s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_ReadChunk{ReadChunk: &pb.ReadChunk{
				RequestId: req.RequestId, Data: data, Eof: isLast, FileSize: fileSize,
			}}})
			if sendErr != nil || isLast {
				return
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				s.sendReadError(req.RequestId, readErr)
			} else if n == 0 {
				_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_ReadChunk{ReadChunk: &pb.ReadChunk{
					RequestId: req.RequestId, Eof: true, FileSize: fileSize,
				}}})
			}
			return
		}
	}
}

func (s *session) handleWriteOpen(req *pb.WriteOpenRequest) {
	resp := &pb.WriteOpenResponse{RequestId: req.RequestId}
	absDir, err := normalizePath(req.DirPath)
	if err != nil {
		resp.Error = err.Error()
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_WriteOpenResponse{WriteOpenResponse: resp}})
		return
	}
	finalPath := filepath.Join(absDir, req.FileName)
	// 与直连 SSH 路径一致：先写临时文件，close 时再原子 rename，避免下载方看到写了一半的文件。
	tempPath := filepath.Join(absDir, "."+req.FileName+".uploading.part")
	f, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		resp.Error = err.Error()
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_WriteOpenResponse{WriteOpenResponse: resp}})
		return
	}
	s.writesMu.Lock()
	s.writes[req.RequestId] = &writeHandle{file: f, tempPath: tempPath, finalPath: finalPath}
	s.writesMu.Unlock()
	_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_WriteOpenResponse{WriteOpenResponse: resp}})
}

func (s *session) handleWriteChunk(req *pb.WriteChunkRequest) {
	s.writesMu.Lock()
	handle := s.writes[req.RequestId]
	s.writesMu.Unlock()

	resp := &pb.WriteChunkAck{RequestId: req.RequestId}
	if handle == nil {
		resp.Error = "write session not found（可能已超时或已关闭）"
	} else if n, err := handle.file.Write(req.Data); err != nil {
		resp.Error = err.Error()
	} else {
		resp.BytesWritten = int64(n)
	}
	_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_WriteChunkAck{WriteChunkAck: resp}})
}

func (s *session) handleWriteClose(req *pb.WriteCloseRequest) {
	s.writesMu.Lock()
	handle := s.writes[req.RequestId]
	delete(s.writes, req.RequestId)
	s.writesMu.Unlock()

	resp := &pb.WriteCloseResponse{RequestId: req.RequestId}
	if handle == nil {
		resp.Error = "write session not found（可能已超时或已关闭）"
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_WriteCloseResponse{WriteCloseResponse: resp}})
		return
	}

	info, statErr := handle.file.Stat()
	_ = handle.file.Close()

	if req.Abort {
		_ = os.Remove(handle.tempPath)
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_WriteCloseResponse{WriteCloseResponse: resp}})
		return
	}

	// 目标路径若已存在旧文件，先删除再 rename，行为与直连 SSH 上传路径保持一致。
	if _, err := os.Stat(handle.finalPath); err == nil {
		_ = os.Remove(handle.finalPath)
	}
	if err := os.Rename(handle.tempPath, handle.finalPath); err != nil {
		resp.Error = err.Error()
		_ = os.Remove(handle.tempPath)
	} else {
		resp.Path = handle.finalPath
		if statErr == nil {
			resp.TotalBytes = info.Size()
		}
	}
	_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_WriteCloseResponse{WriteCloseResponse: resp}})
}

func (s *session) handleRename(req *pb.RenameRequest) {
	resp := &pb.RenameResponse{RequestId: req.RequestId}
	if strings.Contains(req.NewName, "/") {
		resp.Error = "new_name 不能包含路径分隔符"
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_RenameResponse{RenameResponse: resp}})
		return
	}
	absOld, err := normalizePath(req.Path)
	if err != nil {
		resp.Error = err.Error()
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_RenameResponse{RenameResponse: resp}})
		return
	}
	newPath := filepath.Join(filepath.Dir(absOld), req.NewName)
	if err := os.Rename(absOld, newPath); err != nil {
		resp.Error = err.Error()
	} else {
		resp.Path = newPath
		resp.Name = req.NewName
	}
	_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_RenameResponse{RenameResponse: resp}})
}

func (s *session) handleDelete(req *pb.DeleteRequest) {
	resp := &pb.DeleteResponse{RequestId: req.RequestId}
	absPath, err := normalizePath(req.Path)
	if err != nil {
		resp.Error = err.Error()
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_DeleteResponse{DeleteResponse: resp}})
		return
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		resp.Error = err.Error()
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_DeleteResponse{DeleteResponse: resp}})
		return
	}
	if info.IsDir() {
		if !req.Recursive {
			resp.Error = "目录删除需要 recursive=true"
		} else if err := os.RemoveAll(absPath); err != nil {
			resp.Error = err.Error()
		} else {
			resp.Path = absPath
		}
	} else if err := os.Remove(absPath); err != nil {
		resp.Error = err.Error()
	} else {
		resp.Path = absPath
	}
	_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_DeleteResponse{DeleteResponse: resp}})
}

func (s *session) handleMkdir(req *pb.MkdirRequest) {
	resp := &pb.MkdirResponse{RequestId: req.RequestId}
	if strings.Contains(req.Name, "/") {
		resp.Error = "name 不能包含路径分隔符"
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_MkdirResponse{MkdirResponse: resp}})
		return
	}
	absDir, err := normalizePath(req.Path)
	if err != nil {
		resp.Error = err.Error()
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_MkdirResponse{MkdirResponse: resp}})
		return
	}
	newDirPath := filepath.Join(absDir, req.Name)
	if err := os.Mkdir(newDirPath, 0o755); err != nil {
		resp.Error = err.Error()
	} else {
		resp.Path = newDirPath
		resp.Name = req.Name
	}
	_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_MkdirResponse{MkdirResponse: resp}})
}

func (s *session) handleCreateFile(req *pb.CreateFileRequest) {
	resp := &pb.CreateFileResponse{RequestId: req.RequestId}
	if strings.Contains(req.Name, "/") {
		resp.Error = "name 不能包含路径分隔符"
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_CreateFileResponse{CreateFileResponse: resp}})
		return
	}
	absDir, err := normalizePath(req.Path)
	if err != nil {
		resp.Error = err.Error()
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_CreateFileResponse{CreateFileResponse: resp}})
		return
	}
	newFilePath := filepath.Join(absDir, req.Name)
	// O_EXCL：与直连 SSH 路径的 sftp_client.file(path, 'xb') 语义对齐，目标已存在时报错而不是截断覆盖。
	f, err := os.OpenFile(newFilePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		resp.Error = err.Error()
	} else {
		_ = f.Close()
		resp.Path = newFilePath
		resp.Name = req.Name
	}
	_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_CreateFileResponse{CreateFileResponse: resp}})
}
