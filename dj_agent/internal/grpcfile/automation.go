package grpcfile

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"

	"github.com/chlinwei/djadmin/dj_agent/internal/grpcfile/pb"
)

// handleAutomationExecute 处理 backend 经 gRPC 长连接下发的一次自动化任务：
// 复用 agent 内统一执行器 exec.Run（与旧 HTTP /api/v1/automation/execute 走同一逻辑），
// 同步等待结果后回传 AutomationExecuteResponse。任务耗时较长，调用方已在独立 goroutine 中执行。
func (s *session) handleAutomationExecute(ctx context.Context, req *pb.AutomationExecuteRequest) {
	resp := &pb.AutomationExecuteResponse{
		RequestId: req.RequestId,
		JobId:     req.JobId,
	}

	if s.exec == nil {
		resp.Status = string(protocol.StatusFailed)
		resp.ErrorMessage = "agent executor unavailable"
		s.sendAutomationResponse(resp)
		return
	}

	action := strings.TrimSpace(req.Action)
	if action == "get_agent_runtime_status" {
		s.handleAgentRuntimeStatus(resp)
		return
	}
	if action == "" {
		resp.Status = string(protocol.StatusFailed)
		resp.ErrorMessage = "action is required"
		s.sendAutomationResponse(resp)
		return
	}

	var params map[string]any
	if strings.TrimSpace(req.ParamsJson) != "" {
		if err := json.Unmarshal([]byte(req.ParamsJson), &params); err != nil {
			resp.Status = string(protocol.StatusFailed)
			resp.ErrorMessage = fmt.Sprintf("invalid params_json: %v", err)
			s.sendAutomationResponse(resp)
			return
		}
	}

	jobType := protocol.TaskType(strings.TrimSpace(req.Type))
	if jobType == "" {
		jobType = protocol.TaskTypeCustom
	}
	jobID := strings.TrimSpace(req.JobId)
	if jobID == "" {
		jobID = fmt.Sprintf("grpc-run-automation-%d", time.Now().UnixNano())
	}
	timeoutSeconds := int(req.TimeoutSeconds)
	if timeoutSeconds <= 0 {
		timeoutSeconds = 3600
	}

	job := protocol.Job{
		JobID:   jobID,
		Type:    jobType,
		Action:  action,
		Params:  params,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	}

	result, runErr := s.exec.Run(ctx, job)
	resp.JobId = result.JobID
	resp.Status = string(result.Status)
	resp.ExitCode = int32(result.ExitCode)
	resp.Stdout = result.Stdout
	resp.Stderr = result.Stderr
	resp.ErrorMessage = result.Error
	resp.CostMs = result.CostMS
	if runErr != nil && resp.ErrorMessage == "" {
		resp.ErrorMessage = runErr.Error()
	}
	if len(result.Data) > 0 {
		if raw, err := json.Marshal(result.Data); err == nil {
			resp.ResultDataJson = string(raw)
		}
	}

	s.sendAutomationResponse(resp)
}

func (s *session) handleAgentRuntimeStatus(resp *pb.AutomationExecuteResponse) {
	statusData := map[string]any{}
	if s.runtimeStatusProvider != nil {
		if provided := s.runtimeStatusProvider(); provided != nil {
			statusData = provided
		}
	}

	resp.Status = string(protocol.StatusSuccess)
	resp.ExitCode = 0
	resp.Stdout = ""
	resp.Stderr = ""
	resp.ErrorMessage = ""
	resp.CostMs = 0

	raw, err := json.Marshal(statusData)
	if err != nil {
		resp.Status = string(protocol.StatusFailed)
		resp.ErrorMessage = fmt.Sprintf("encode runtime status failed: %v", err)
		s.sendAutomationResponse(resp)
		return
	}
	resp.ResultDataJson = string(raw)
	s.sendAutomationResponse(resp)
}

func (s *session) sendAutomationResponse(resp *pb.AutomationExecuteResponse) {
	if err := s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_AutomationExecuteResponse{AutomationExecuteResponse: resp}}); err != nil {
		slog.Warn("send automation execute response failed", "request_id", resp.RequestId, "err", err)
	}
}
