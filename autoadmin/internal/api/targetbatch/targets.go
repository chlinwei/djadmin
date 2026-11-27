// Package targetbatch describes per-host outcomes shared by exporter and log collection targets.
package targetbatch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"autoadmin/internal/agent"
	"autoadmin/internal/agent/pb"
	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func Create(hostID int64, name, ip string, deleted bool, err error, insert func() (int64, bool, error)) (response.BatchResult, int64) {
	label := name
	if label == "" {
		label = ip
	}
	if label == "" {
		label = fmt.Sprintf("host-%d", hostID)
	}
	var targetID int64
	if errors.Is(err, sql.ErrNoRows) || (err == nil && deleted) {
		err = apperror.New(apperror.CodeNotFound, "host not found")
	}
	if err == nil {
		var created bool
		targetID, created, err = insert()
		if err == nil && !created {
			err = apperror.New(apperror.CodeInvalidArgument, "target already managed")
		}
	}
	item := response.BatchItem(hostID, err)
	item.HostID, item.Host = hostID, label
	if item.OK {
		item.Detail = gin.H{"created": true, "target_id": targetID}
	}
	return item, targetID
}

// Install records acceptance, not execution. A failed dispatch cannot undo the committed insert.
func Install(item *response.BatchResult, targetID, jobID int64, err error) {
	result := response.BatchItem(item.ID, err)
	detail := gin.H{"created": true, "target_id": targetID, "install_accepted": result.OK}
	if jobID > 0 {
		detail["job_id"] = jobID
	}
	result.Host, result.HostID, result.Detail = item.Host, item.HostID, detail
	if result.OK {
		result.Message = "install job dispatched"
	}
	*item = result
}

func Report(ctx *gin.Context, results []response.BatchResult, extra gin.H) {
	if extra == nil {
		extra = gin.H{}
	}

	success := 0
	for _, item := range results {
		if item.OK {
			success++
		}
	}
	extra["total"], extra["success"], extra["failed"] = len(results), success, len(results)-success
	response.BatchWith(ctx, results, extra)
}

func ServiceDispatchError(err error) error {
	switch {
	case errors.Is(err, agent.ErrAgentOffline):
		return apperror.New(apperror.CodeInvalidArgument, "host agent is offline")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return apperror.New(apperror.CodeInvalidArgument, err.Error())
	default:
		return apperror.WithCause(apperror.New(apperror.CodeInternal, "无法完成主机 Agent 服务控制请求"), err)
	}
}

// A status probe may legitimately return a nonzero exit code (e.g. inactive).
// Start/stop must report actual execution failures, retaining the agent job ID and output.
func ServiceResult(result *pb.AutomationExecuteResponse, action, service string) (gin.H, error) {
	detail := gin.H{"job_id": result.JobId, "status": result.Status, "exit_code": result.ExitCode, "stdout": result.Stdout, "stderr": result.Stderr, "error_message": result.ErrorMessage}
	if action != "status" && (result.Status != "success" || result.ExitCode != 0) {
		reason := fmt.Sprintf("status=%q exit_code=%d", result.Status, result.ExitCode)
		for _, text := range []string{result.ErrorMessage, result.Stderr, result.Stdout} {
			if strings.TrimSpace(text) != "" {
				reason = strings.TrimSpace(text)
				break
			}
		}
		return detail, apperror.New(apperror.CodeInvalidArgument, fmt.Sprintf("systemctl %s %s.service failed: %s", action, service, reason))
	}
	return detail, nil
}
