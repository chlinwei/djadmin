package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/config"
	"github.com/chlinwei/djadmin/dj_agent/internal/executor"
	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
	"github.com/nats-io/nats.go"
)

type App struct {
	cfg                          config.Config
	hostReportIntervalUpdateChan chan time.Duration
	httpServer                   *http.Server
}

const minHostReportInterval = 30 * time.Second
const maxHostReportInterval = 12 * time.Hour

type natsCommand struct {
	CommandID       string            `json:"command_id"`
	ClientRequestID string            `json:"client_request_id"`
	JobID           string            `json:"job_id"`
	TargetType      string            `json:"target_type"`
	TargetValue     string            `json:"target_value"`
	Type            string            `json:"type"`
	Action          string            `json:"action"`
	Command         string            `json:"command"`
	Args            []string          `json:"args"`
	WorkDir         string            `json:"work_dir"`
	Env             map[string]string `json:"env"`
	Params          map[string]any    `json:"params"`
	TimeoutSeconds  int               `json:"timeout_seconds"`
}

func New(cfg config.Config) *App {
	return &App{cfg: cfg}
}

func (a *App) Run() error {
	slog.Info("app run begin", "agent_id", a.cfg.AgentID)
	exec := executor.New(0)
	natsURL := strings.TrimSpace(os.Getenv("DJ_AGENT_NATS_URL"))
	if natsURL == "" {
		natsURL = "nats://127.0.0.1:4222"
	}

	nc, err := nats.Connect(natsURL, nats.Name("dj-agent-"+a.cfg.AgentID))
	if err != nil {
		return fmt.Errorf("connect nats failed: %w", err)
	}
	defer nc.Close()
	termMgr := newTerminalManager(a.cfg.AgentID, nc)
	queueGroup := fmt.Sprintf("agent.%s", a.cfg.AgentID)

	subjects := a.buildCommandSubjects()
	for _, subject := range subjects {
		if _, subErr := nc.QueueSubscribe(subject, queueGroup, a.handleNATSCommand(exec, nc)); subErr != nil {
			return fmt.Errorf("subscribe subject %s failed: %w", subject, subErr)
		}
	}
	if _, subErr := nc.QueueSubscribe(fmt.Sprintf("cmd.term.%s", a.cfg.AgentID), queueGroup, a.handleTermCommand(termMgr)); subErr != nil {
		return fmt.Errorf("subscribe term subject failed: %w", subErr)
	}
	if err = nc.Flush(); err != nil {
		return fmt.Errorf("flush subscriptions failed: %w", err)
	}
	slog.Info("nats subscriptions ready", "agent_id", a.cfg.AgentID, "subjects", subjects)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	httpErrCh := make(chan error, 1)
	a.startHTTPServer(exec, httpErrCh)
	heartbeatTicker := time.NewTicker(10 * time.Second)
	defer heartbeatTicker.Stop()
	a.hostReportIntervalUpdateChan = make(chan time.Duration, 1)
	hostReportInterval := a.resolveHostReportInterval()
	hostReportTicker := time.NewTicker(hostReportInterval)
	defer hostReportTicker.Stop()

	// 启动后立即上报一次主机快照，避免等待首个周期导致版本延迟。
	a.publishHostSnapshot(exec, nc)

	for {
		select {
		case serverErr := <-httpErrCh:
			return serverErr
		case <-ctx.Done():
			slog.Info("shutdown signal received", "agent_id", a.cfg.AgentID)
			termMgr.CloseAll()

			shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
			defer cancel()

			return a.gracefulShutdown(shutdownCtx)
		case <-heartbeatTicker.C:
			a.publishHeartbeat(nc)
		case <-hostReportTicker.C:
			a.publishHostSnapshot(exec, nc)
		case nextInterval := <-a.hostReportIntervalUpdateChan:
			if nextInterval == hostReportInterval {
				continue
			}
			hostReportInterval = nextInterval
			hostReportTicker.Reset(hostReportInterval)
			slog.Info("host report interval updated by nats command", "interval", hostReportInterval.String())
		}
	}
}

func (a *App) handleTermCommand(termMgr *terminalManager) nats.MsgHandler {
	return func(msg *nats.Msg) {
		var command termCommand
		if err := json.Unmarshal(msg.Data, &command); err != nil {
			slog.Error("decode term command failed", "subject", msg.Subject, "err", err)
			return
		}
		termMgr.Handle(command)
	}
}

func (a *App) resolveHostReportInterval() time.Duration {
	defaultInterval := a.cfg.HostReportInterval
	interval, err := a.fetchHostReportIntervalFromBackend(defaultInterval)
	if err != nil {
		slog.Warn(
			"fetch host report interval from backend failed, fallback to local config",
			"err", err,
			"interval", defaultInterval.String(),
		)
		return defaultInterval
	}

	slog.Info("resolved host report interval from backend", "interval", interval.String())
	return interval
}

func (a *App) fetchHostReportIntervalFromBackend(defaultInterval time.Duration) (time.Duration, error) {
	baseURL := strings.TrimSpace(a.cfg.BackendBaseURL)
	token := strings.TrimSpace(a.cfg.BackendToken)
	if baseURL == "" {
		return 0, fmt.Errorf("backend base url is empty")
	}
	if token == "" {
		return 0, fmt.Errorf("backend token is empty")
	}

	endpoint := fmt.Sprintf("%s/api/agent/configs/by-key/%s", strings.TrimRight(baseURL, "/"), "sys.assets.collect.interval_seconds")
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("build request failed: %w", err)
	}
	req.Header.Set("Authorization", token)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request backend failed: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return 0, fmt.Errorf("read backend response failed: %w", readErr)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("backend response status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Value any `json:"value"`
		} `json:"data"`
	}
	if err = json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("decode backend response failed: %w", err)
	}
	if payload.Code != 200 {
		return 0, fmt.Errorf("backend business error code=%d msg=%s", payload.Code, payload.Msg)
	}

	seconds, err := parsePositiveInt(payload.Data.Value)
	if err != nil {
		return 0, fmt.Errorf("invalid backend interval value: %w", err)
	}
	interval := time.Duration(seconds) * time.Second
	if interval <= 0 {
		return 0, fmt.Errorf("invalid backend interval <= 0")
	}

	// 保底保护，避免异常配置把上报打成高频风暴。
	if interval < minHostReportInterval || interval > maxHostReportInterval {
		return defaultInterval, nil
	}
	return interval, nil
}

func parsePositiveInt(value any) (int, error) {
	switch v := value.(type) {
	case float64:
		if v <= 0 {
			return 0, fmt.Errorf("value must be > 0")
		}
		return int(v), nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || parsed <= 0 {
			return 0, fmt.Errorf("value must be positive integer")
		}
		return parsed, nil
	case int:
		if v <= 0 {
			return 0, fmt.Errorf("value must be > 0")
		}
		return v, nil
	default:
		return 0, fmt.Errorf("unsupported value type %T", value)
	}
}

func (a *App) buildCommandSubjects() []string {
	subjects := []string{fmt.Sprintf("cmd.agent.%s", a.cfg.AgentID), "cmd.all"}
	groupNames := strings.TrimSpace(os.Getenv("DJ_AGENT_GROUPS"))
	if groupNames == "" {
		return subjects
	}
	for _, group := range strings.Split(groupNames, ",") {
		name := strings.TrimSpace(group)
		if name == "" {
			continue
		}
		subjects = append(subjects, fmt.Sprintf("cmd.group.%s", name))
	}
	return subjects
}

func (a *App) handleNATSCommand(exec *executor.Executor, nc *nats.Conn) nats.MsgHandler {
	return func(msg *nats.Msg) {
		ctx := context.Background()
		var command natsCommand
		if err := json.Unmarshal(msg.Data, &command); err != nil {
			slog.Error("decode nats command failed", "subject", msg.Subject, "err", err)
			return
		}

		job := protocol.Job{
			JobID:   strings.TrimSpace(command.JobID),
			Type:    protocol.TaskType(strings.TrimSpace(command.Type)),
			Action:  strings.TrimSpace(command.Action),
			Command: strings.TrimSpace(command.Command),
			Args:    command.Args,
			WorkDir: strings.TrimSpace(command.WorkDir),
			Env:     command.Env,
			Params:  command.Params,
			Timeout: time.Duration(command.TimeoutSeconds) * time.Second,
		}

		if job.JobID == "" || job.Type == "" || job.Action == "" {
			slog.Warn("ignore invalid nats command", "subject", msg.Subject, "job_id", job.JobID, "type", job.Type, "action", job.Action)
			return
		}

		if strings.EqualFold(job.Action, "set_host_report_interval") {
			a.publishJobEvent(nc, job, protocol.StatusRunning, "")
			nextInterval, updateErr := a.resolveHostReportIntervalFromCommand(job.Params)
			if updateErr != nil {
				result := protocol.JobResult{
					JobID:  job.JobID,
					Type:   job.Type,
					Action: job.Action,
					Status: protocol.StatusFailed,
					Error:  updateErr.Error(),
				}
				a.publishJobResult(nc, result)
				slog.Warn("set_host_report_interval failed", "agent_id", a.cfg.AgentID, "job_id", job.JobID, "err", updateErr)
				return
			}

			a.pushHostReportIntervalUpdate(nextInterval)
			result := protocol.JobResult{
				JobID:    job.JobID,
				Type:     job.Type,
				Action:   job.Action,
				Status:   protocol.StatusSuccess,
				ExitCode: 0,
				Data: map[string]any{
					"interval_seconds": int(nextInterval / time.Second),
				},
			}
			a.publishJobResult(nc, result)
			slog.Info("set_host_report_interval accepted", "agent_id", a.cfg.AgentID, "job_id", job.JobID, "interval", nextInterval.String())
			return
		}

		a.publishJobEvent(nc, job, protocol.StatusRunning, "")
		result, runErr := exec.Run(ctx, job)
		a.publishJobResult(nc, result)
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			slog.Warn("job execution finished with error", "agent_id", a.cfg.AgentID, "job_id", job.JobID, "err", runErr)
			return
		}
		slog.Info("job execution finished", "agent_id", a.cfg.AgentID, "job_id", job.JobID, "status", result.Status)
	}
}

func (a *App) resolveHostReportIntervalFromCommand(params map[string]any) (time.Duration, error) {
	rawValue, ok := params["interval_seconds"]
	if !ok {
		rawValue = params["value"]
	}
	if rawValue == nil {
		return 0, fmt.Errorf("params.interval_seconds is required")
	}

	seconds, err := parsePositiveInt(rawValue)
	if err != nil {
		return 0, fmt.Errorf("invalid interval_seconds: %w", err)
	}
	interval := time.Duration(seconds) * time.Second
	if interval < minHostReportInterval {
		return 0, fmt.Errorf("interval_seconds must be >= %d", int(minHostReportInterval/time.Second))
	}
	if interval > maxHostReportInterval {
		return 0, fmt.Errorf("interval_seconds must be <= %d", int(maxHostReportInterval/time.Second))
	}
	return interval, nil
}

func (a *App) pushHostReportIntervalUpdate(next time.Duration) {
	if a.hostReportIntervalUpdateChan == nil {
		return
	}
	select {
	case a.hostReportIntervalUpdateChan <- next:
	default:
		select {
		case <-a.hostReportIntervalUpdateChan:
		default:
		}
		a.hostReportIntervalUpdateChan <- next
	}
}

func (a *App) publishHeartbeat(nc *nats.Conn) {
	payload := map[string]any{
		"agent_id": a.cfg.AgentID,
		"ts":       time.Now().UTC().Format(time.RFC3339),
	}
	a.publishJSON(nc, fmt.Sprintf("hb.agent.%s", a.cfg.AgentID), payload)
}

func (a *App) publishHostSnapshot(exec *executor.Executor, nc *nats.Conn) {
	job := protocol.Job{
		JobID:  fmt.Sprintf("periodic-host-info-%d", time.Now().Unix()),
		Type:   protocol.TaskTypeInventory,
		Action: "get_host_info",
	}

	result, runErr := exec.Run(context.Background(), job)
	payload := map[string]any{
		"agent_id":      a.cfg.AgentID,
		"action":        "get_host_info",
		"status":        result.Status,
		"result_data":   result.Data,
		"error_message": result.Error,
		"ts":            time.Now().UTC().Format(time.RFC3339),
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		payload["status"] = protocol.StatusFailed
		payload["error_message"] = runErr.Error()
	}
	a.publishJSON(nc, fmt.Sprintf("rpt.host.%s", a.cfg.AgentID), payload)
}

func (a *App) publishJobEvent(nc *nats.Conn, job protocol.Job, status protocol.JobStatus, errMsg string) {
	payload := map[string]any{
		"job_id":   job.JobID,
		"agent_id": a.cfg.AgentID,
		"action":   job.Action,
		"status":   status,
		"error":    errMsg,
		"ts":       time.Now().UTC().Format(time.RFC3339),
	}
	a.publishJSON(nc, fmt.Sprintf("evt.job.%s.%s", job.JobID, status), payload)
}

func (a *App) publishJobResult(nc *nats.Conn, result protocol.JobResult) {
	payload := map[string]any{
		"job_id":        result.JobID,
		"agent_id":      a.cfg.AgentID,
		"action":        result.Action,
		"status":        result.Status,
		"retcode":       result.ExitCode,
		"stdout":        result.Stdout,
		"stderr":        result.Stderr,
		"result_data":   result.Data,
		"error_message": result.Error,
		"ts":            time.Now().UTC().Format(time.RFC3339),
	}
	a.publishJSON(nc, fmt.Sprintf("ret.job.%s", result.JobID), payload)
}

func (a *App) publishJSON(nc *nats.Conn, subject string, payload map[string]any) {
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("marshal nats payload failed", "subject", subject, "err", err)
		return
	}
	if err = nc.Publish(subject, body); err != nil {
		slog.Error("publish nats message failed", "subject", subject, "err", err)
	}
}

func (a *App) gracefulShutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := a.stopHTTPServer(ctx); err != nil {
		return err
	}
	return nil
}
