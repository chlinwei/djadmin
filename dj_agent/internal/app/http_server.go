package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/executor"
	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

type httpExecuteRequest struct {
	JobID          string         `json:"job_id"`
	Type           string         `json:"type"`
	Action         string         `json:"action"`
	Params         map[string]any `json:"params"`
	TimeoutSeconds int            `json:"timeout_seconds"`
}

type httpExecuteResponse struct {
	Code int            `json:"code"`
	Msg  string         `json:"msg"`
	Data map[string]any `json:"data,omitempty"`
	Err  string         `json:"error_message,omitempty"`
}

func (a *App) startHTTPServer(exec *executor.Executor, errCh chan<- error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(httpExecuteResponse{Code: 200, Msg: "ok", Data: map[string]any{"agent_id": a.cfg.AgentID}})
	})
	mux.HandleFunc("/api/v1/agent/status", a.handleAgentStatus())
	mux.HandleFunc("/api/v1/automation/execute", a.handleAutomationExecute(exec))

	a.httpServer = &http.Server{
		Addr:              a.cfg.HTTPListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("agent http server started", "addr", a.cfg.HTTPListenAddr)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("agent http server failed: %w", err)
		}
	}()
}

func (a *App) handleAgentStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			a.writeHTTPJSON(w, http.StatusMethodNotAllowed, httpExecuteResponse{Code: 405, Msg: "method not allowed"})
			return
		}
		if !a.checkHTTPAuth(r) {
			a.writeHTTPJSON(w, http.StatusUnauthorized, httpExecuteResponse{Code: 401, Msg: "unauthorized"})
			return
		}

		a.writeHTTPJSON(w, http.StatusOK, httpExecuteResponse{
			Code: 200,
			Msg:  "success",
			Data: a.getRuntimeStatusData(),
		})
	}
}

func (a *App) stopHTTPServer(ctx context.Context) error {
	if a.httpServer == nil {
		return nil
	}
	return a.httpServer.Shutdown(ctx)
}

func (a *App) handleAutomationExecute(exec *executor.Executor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			a.writeHTTPJSON(w, http.StatusMethodNotAllowed, httpExecuteResponse{Code: 405, Msg: "method not allowed"})
			return
		}
		if !a.checkHTTPAuth(r) {
			a.writeHTTPJSON(w, http.StatusUnauthorized, httpExecuteResponse{Code: 401, Msg: "unauthorized"})
			return
		}

		var req httpExecuteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			a.writeHTTPJSON(w, http.StatusBadRequest, httpExecuteResponse{Code: 400, Msg: "invalid json", Err: err.Error()})
			return
		}

		action := strings.TrimSpace(req.Action)
		if action != "run_automation_task" {
			a.writeHTTPJSON(w, http.StatusBadRequest, httpExecuteResponse{Code: 400, Msg: "unsupported action", Err: "action must be run_automation_task"})
			return
		}

		jobType := protocol.TaskType(strings.TrimSpace(req.Type))
		if jobType == "" {
			jobType = protocol.TaskTypeCustom
		}
		jobID := strings.TrimSpace(req.JobID)
		if jobID == "" {
			jobID = fmt.Sprintf("http-run-automation-%d", time.Now().UnixNano())
		}
		timeoutSeconds := req.TimeoutSeconds
		if timeoutSeconds <= 0 {
			timeoutSeconds = 3600
		}

		job := protocol.Job{
			JobID:   jobID,
			Type:    jobType,
			Action:  action,
			Params:  req.Params,
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		}

		result, runErr := exec.Run(r.Context(), job)
		statusCode := http.StatusOK
		msg := "success"
		if runErr != nil {
			msg = "failed"
		}
		a.writeHTTPJSON(w, statusCode, httpExecuteResponse{
			Code: 200,
			Msg:  msg,
			Data: map[string]any{
				"job_id":        result.JobID,
				"status":        result.Status,
				"exit_code":     result.ExitCode,
				"stdout":        result.Stdout,
				"stderr":        result.Stderr,
				"result_data":   result.Data,
				"error_message": result.Error,
				"cost_ms":       result.CostMS,
			},
			Err: result.Error,
		})
	}
}

func (a *App) checkHTTPAuth(r *http.Request) bool {
	token := strings.TrimSpace(a.cfg.HTTPAuthToken)
	if token == "" {
		return true
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	xToken := strings.TrimSpace(r.Header.Get("X-Agent-Token"))
	if xToken != "" && xToken == token {
		return true
	}
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:]) == token
	}
	return false
}

func (a *App) writeHTTPJSON(w http.ResponseWriter, statusCode int, payload httpExecuteResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
