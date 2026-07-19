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
	"sync"
	"syscall"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/config"
	"github.com/chlinwei/djadmin/dj_agent/internal/executor"
	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
	"github.com/rabbitmq/amqp091-go"
)

type App struct {
	cfg                          config.Config
	hostReportIntervalUpdateChan chan time.Duration
	httpServer                   *http.Server
	rabbitmqChannel              *amqp091.Channel
	statusMu                     sync.RWMutex
	startedAt                    time.Time
	heartbeatInterval            time.Duration
	currentHostReportInterval    time.Duration
	lastHeartbeatAt              time.Time
	lastHeartbeatStatus          string
	lastHeartbeatError           string
	lastHostSnapshotAt           time.Time
	lastHostSnapshotStatus       string
	lastHostSnapshotError        string
	isRunning                    bool
}

const minHostReportInterval = 30 * time.Second
const maxHostReportInterval = 12 * time.Hour

// 从RabbitMQ消费的任务命令结构（与Backend发送的相同）
type rabbitCommand struct {
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
	return &App{
		cfg:               cfg,
		heartbeatInterval: 10 * time.Second,
	}
}

func (a *App) Run() error {
	slog.Info("app run begin", "agent_id", a.cfg.AgentID)
	a.markStarted()
	exec := executor.New(0)

	// 连接 RabbitMQ - 用于任务下发、终端命令、上报
	rabbitmqURL := strings.TrimSpace(os.Getenv("DJ_AGENT_RABBITMQ_URL"))
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://admin:admin123@127.0.0.1:5672//"
	}

	conn, err := amqp091.Dial(rabbitmqURL)
	if err != nil {
		return fmt.Errorf("connect rabbitmq failed: %w", err)
	}
	defer conn.Close()

	channel, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("create rabbitmq channel failed: %w", err)
	}
	a.rabbitmqChannel = channel // 保存 channel 供 reportToBackend 使用
	// 不在此处 defer 关闭，让 channel 保持打开供主循环使用
	defer func() {
		if a.rabbitmqChannel != nil {
			a.rabbitmqChannel.Close()
		}
	}() // 在程序退出时才关闭

	// 声明队列
	tasksQueue := "agent.tasks"
	if _, err := channel.QueueDeclare(tasksQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare tasks queue failed: %w", err)
	}

	// 声明上报队列
	reportsQueue := "agent.reports"
	if _, err := channel.QueueDeclare(reportsQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare reports queue failed: %w", err)
	}

	// 声明终端命令队列 - 每个 agent 有独立队列
	termQueue := fmt.Sprintf("agent.term.%s", a.cfg.AgentID)
	if _, err := channel.QueueDeclare(termQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare term queue failed: %w", err)
	}

	// 初始化终端管理器（当前通过 RabbitMQ 上报事件）
	termMgr := newTerminalManager(a.cfg.AgentID)
	// 设置终端事件上报回调
	termMgr.reportFunc = func(eventType string, payload map[string]any) {
		_ = a.reportToBackend("term_event", payload)
	}

	// 启动背景服务
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	httpErrCh := make(chan error, 1)
	a.startHTTPServer(exec, httpErrCh)

	// 定时器
	heartbeatTicker := time.NewTicker(10 * time.Second)
	defer heartbeatTicker.Stop()

	a.hostReportIntervalUpdateChan = make(chan time.Duration, 1)
	hostReportInterval := a.resolveHostReportInterval()
	a.setCurrentHostReportInterval(hostReportInterval)
	hostReportTicker := time.NewTicker(hostReportInterval)
	defer hostReportTicker.Stop()

	// 启动后立即上报一次主机快照
	a.markHostSnapshotTick(time.Now())
	initialSnapshotPayload := a.buildHostSnapshot(exec)
	initialSnapshotStatus := strings.TrimSpace(fmt.Sprintf("%v", initialSnapshotPayload["status"]))
	initialSnapshotError := strings.TrimSpace(fmt.Sprintf("%v", initialSnapshotPayload["error"]))
	a.markHostSnapshotResult(initialSnapshotStatus, initialSnapshotError)
	if err = a.reportToBackend("host_snapshot", initialSnapshotPayload); err != nil {
		a.markHostSnapshotResult("failed", err.Error())
	}

	// 启动 RabbitMQ 任务 consumer goroutine
	taskConsumerErrCh := make(chan error, 1)
	go a.taskConsumer(ctx, channel, exec, taskConsumerErrCh)

	// 启动 RabbitMQ 终端命令 consumer goroutine
	termConsumerErrCh := make(chan error, 1)
	go a.terminalCommandConsumer(ctx, channel, termMgr, termConsumerErrCh)

	// 启动准备完成后立即上报一次上线事件，避免等待首个 ticker 周期。
	_ = a.reportToBackend("agent_status", map[string]any{
		"status": "online",
		"reason": "startup",
	})
	slog.Info("startup status event published", "agent_id", a.cfg.AgentID)

	for {
		select {
		case serverErr := <-httpErrCh:
			return serverErr
		case taskErr := <-taskConsumerErrCh:
			if taskErr != nil && !errors.Is(taskErr, context.Canceled) {
				slog.Error("task consumer error", "err", taskErr)
				// 不直接返回，继续运行以保证终端命令仍可用
			}
		case termErr := <-termConsumerErrCh:
			if termErr != nil && !errors.Is(termErr, context.Canceled) {
				slog.Error("terminal consumer error", "err", termErr)
				// 不直接返回，继续运行以保证任务仍可用
			}
		case <-ctx.Done():
			slog.Info("shutdown signal received", "agent_id", a.cfg.AgentID)
			a.markStopped()
			termMgr.CloseAll()

			shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
			defer cancel()

			return a.gracefulShutdown(shutdownCtx)
		case <-heartbeatTicker.C:
			a.markHeartbeatTick(time.Now())
			if err = a.reportToBackend("heartbeat", map[string]any{
				"agent_id": a.cfg.AgentID,
				"ts":       time.Now().UTC().Format(time.RFC3339),
			}); err != nil {
				a.markHeartbeatResult("failed", err.Error())
			} else {
				a.markHeartbeatResult("success", "")
			}
		case <-hostReportTicker.C:
			a.markHostSnapshotTick(time.Now())
			snapshotPayload := a.buildHostSnapshot(exec)
			snapshotStatus := strings.TrimSpace(fmt.Sprintf("%v", snapshotPayload["status"]))
			snapshotError := strings.TrimSpace(fmt.Sprintf("%v", snapshotPayload["error"]))
			a.markHostSnapshotResult(snapshotStatus, snapshotError)
			if err = a.reportToBackend("host_snapshot", snapshotPayload); err != nil {
				a.markHostSnapshotResult("failed", err.Error())
			}
		case nextInterval := <-a.hostReportIntervalUpdateChan:
			if nextInterval == hostReportInterval {
				continue
			}
			hostReportInterval = nextInterval
			a.setCurrentHostReportInterval(hostReportInterval)
			hostReportTicker.Reset(hostReportInterval)
			slog.Info("host report interval updated", "interval", hostReportInterval.String())
		}
	}
}

func (a *App) markStarted() {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.startedAt = time.Now()
	a.isRunning = true
	a.lastHeartbeatAt = time.Time{}
	a.lastHeartbeatStatus = ""
	a.lastHeartbeatError = ""
	a.lastHostSnapshotAt = time.Time{}
	a.lastHostSnapshotStatus = ""
	a.lastHostSnapshotError = ""
}

func (a *App) markStopped() {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.isRunning = false
}

func (a *App) setCurrentHostReportInterval(interval time.Duration) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.currentHostReportInterval = interval
}

func (a *App) markHeartbeatTick(ts time.Time) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.lastHeartbeatAt = ts
}

func (a *App) markHostSnapshotTick(ts time.Time) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.lastHostSnapshotAt = ts
}

func (a *App) markHeartbeatResult(status string, errText string) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.lastHeartbeatStatus = strings.TrimSpace(status)
	a.lastHeartbeatError = strings.TrimSpace(errText)
}

func (a *App) markHostSnapshotResult(status string, errText string) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.lastHostSnapshotStatus = strings.TrimSpace(status)
	a.lastHostSnapshotError = strings.TrimSpace(errText)
}

func (a *App) getRuntimeStatusData() map[string]any {
	a.statusMu.RLock()
	startedAt := a.startedAt
	isRunning := a.isRunning
	heartbeatInterval := a.heartbeatInterval
	hostReportInterval := a.currentHostReportInterval
	lastHeartbeatAt := a.lastHeartbeatAt
	lastHeartbeatStatus := strings.TrimSpace(a.lastHeartbeatStatus)
	lastHeartbeatError := strings.TrimSpace(a.lastHeartbeatError)
	lastHostSnapshotAt := a.lastHostSnapshotAt
	lastHostSnapshotStatus := strings.TrimSpace(a.lastHostSnapshotStatus)
	lastHostSnapshotError := strings.TrimSpace(a.lastHostSnapshotError)
	a.statusMu.RUnlock()

	now := time.Now()
	if hostReportInterval <= 0 {
		hostReportInterval = a.cfg.HostReportInterval
	}

	resolveNextRunAt := func(lastRunAt time.Time, interval time.Duration) string {
		if interval <= 0 {
			return ""
		}
		if !lastRunAt.IsZero() {
			return lastRunAt.Add(interval).UTC().Format(time.RFC3339)
		}
		return now.Add(interval).UTC().Format(time.RFC3339)
	}

	toRFC3339 := func(ts time.Time) string {
		if ts.IsZero() {
			return ""
		}
		return ts.UTC().Format(time.RFC3339)
	}

	uptimeSeconds := 0
	if !startedAt.IsZero() {
		uptimeSeconds = int(now.Sub(startedAt).Seconds())
		if uptimeSeconds < 0 {
			uptimeSeconds = 0
		}
	}

	registeredTasks := []map[string]any{
		{
			"name":             "heartbeat",
			"task_type":        "periodic",
			"source":           "builtin",
			"enabled":          true,
			"status":           "running",
			"job_id":           "",
			"command_id":       "",
			"interval_seconds": int(heartbeatInterval.Seconds()),
			"last_run_at":      toRFC3339(lastHeartbeatAt),
			"next_run_at":      resolveNextRunAt(lastHeartbeatAt, heartbeatInterval),
			"updated_at":       toRFC3339(lastHeartbeatAt),
			"error":            lastHeartbeatError,
			"last_result":      lastHeartbeatStatus,
		},
		{
			"name":             "host_snapshot",
			"task_type":        "periodic",
			"source":           "builtin",
			"enabled":          true,
			"status":           "running",
			"job_id":           "",
			"command_id":       "",
			"interval_seconds": int(hostReportInterval.Seconds()),
			"last_run_at":      toRFC3339(lastHostSnapshotAt),
			"next_run_at":      resolveNextRunAt(lastHostSnapshotAt, hostReportInterval),
			"updated_at":       toRFC3339(lastHostSnapshotAt),
			"error":            lastHostSnapshotError,
			"last_result":      lastHostSnapshotStatus,
		},
	}

	return map[string]any{
		"agent_id": a.cfg.AgentID,
		"version":  "dev",
		"process": map[string]any{
			"pid":            os.Getpid(),
			"running":        isRunning,
			"started_at":     toRFC3339(startedAt),
			"uptime_seconds": uptimeSeconds,
		},
		"http": map[string]any{
			"listen_addr":  a.cfg.HTTPListenAddr,
			"auth_enabled": strings.TrimSpace(a.cfg.HTTPAuthToken) != "",
		},
		"config": map[string]any{
			"backend_base_url":                      a.cfg.BackendBaseURL,
			"max_workers":                           a.cfg.MaxWorkers,
			"shutdown_timeout_seconds":              int(a.cfg.ShutdownTimeout.Seconds()),
			"host_report_interval_fallback_raw":     a.cfg.HostReportIntervalRaw,
			"host_report_interval_fallback_seconds": int(a.cfg.HostReportInterval.Seconds()),
			"host_report_interval_current_seconds":  int(hostReportInterval.Seconds()),
		},
		"schedulers": map[string]any{
			"heartbeat": map[string]any{
				"enabled":          true,
				"interval_seconds": int(heartbeatInterval.Seconds()),
				"last_run_at":      toRFC3339(lastHeartbeatAt),
				"next_run_at":      resolveNextRunAt(lastHeartbeatAt, heartbeatInterval),
			},
			"host_snapshot": map[string]any{
				"enabled":          true,
				"interval_seconds": int(hostReportInterval.Seconds()),
				"last_run_at":      toRFC3339(lastHostSnapshotAt),
				"next_run_at":      resolveNextRunAt(lastHostSnapshotAt, hostReportInterval),
			},
		},
		"runtime": map[string]any{
			"mq_connected": a.rabbitmqChannel != nil,
		},
		"registered_tasks": registeredTasks,
	}
}

// RabbitMQ consumer - 从agent.tasks队列消费任务
func (a *App) taskConsumer(ctx context.Context, channel *amqp091.Channel, exec *executor.Executor, errCh chan error) {
	msgs, err := channel.Consume(
		"agent.tasks", // queue
		"",            // consumer
		false,         // auto-ack（手动确认）
		false,         // exclusive
		false,         // noLocal
		false,         // noWait
		nil,           // args
	)
	if err != nil {
		errCh <- fmt.Errorf("consume tasks queue failed: %w", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			errCh <- nil
			return
		case delivery := <-msgs:
			if delivery.Body == nil {
				continue
			}

			var command rabbitCommand
			if err := json.Unmarshal(delivery.Body, &command); err != nil {
				slog.Error("decode rabbitmq command failed", "err", err)
				delivery.Nack(false, false) // 无法解析，不重新入队
				continue
			}

			a.handleCommand(exec, command)
			delivery.Ack(false) // 处理完确认
		}
	}
}

// terminalCommandConsumer - 从 RabbitMQ 终端队列消费终端命令
func (a *App) terminalCommandConsumer(ctx context.Context, channel *amqp091.Channel, termMgr *terminalManager, errCh chan error) {
	termQueue := fmt.Sprintf("agent.term.%s", a.cfg.AgentID)
	msgs, err := channel.Consume(
		termQueue, // queue
		"",        // consumer
		false,     // auto-ack（手动确认）
		false,     // exclusive
		false,     // noLocal
		false,     // noWait
		nil,       // args
	)
	if err != nil {
		errCh <- fmt.Errorf("consume term queue failed: %w", err)
		return
	}

	slog.Info("terminal consumer ready", "agent_id", a.cfg.AgentID, "queue", termQueue)

	for {
		select {
		case <-ctx.Done():
			errCh <- nil
			return
		case delivery := <-msgs:
			if delivery.Body == nil {
				continue
			}

			var cmd termCommand
			if err := json.Unmarshal(delivery.Body, &cmd); err != nil {
				slog.Error("decode terminal command failed", "err", err)
				delivery.Nack(false, false) // 无法解析，不重新入队
				continue
			}

			termMgr.Handle(cmd)
			delivery.Ack(false) // 处理完确认
		}
	}
}

// 处理来自RabbitMQ的任务命令
func (a *App) handleCommand(exec *executor.Executor, command rabbitCommand) {
	ctx := context.Background()

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
		slog.Warn("ignore invalid command", "job_id", job.JobID, "type", job.Type, "action", job.Action)
		return
	}

	// 特殊处理：设置主机上报间隔
	if strings.EqualFold(job.Action, "set_host_report_interval") {
		_ = a.reportToBackend("job_event", map[string]any{
			"job_id":   job.JobID,
			"agent_id": a.cfg.AgentID,
			"action":   job.Action,
			"status":   protocol.StatusRunning,
			"error":    "",
			"ts":       time.Now().UTC().Format(time.RFC3339),
		})

		nextInterval, updateErr := a.resolveHostReportIntervalFromCommand(job.Params)
		if updateErr != nil {
			result := protocol.JobResult{
				JobID:  job.JobID,
				Type:   job.Type,
				Action: job.Action,
				Status: protocol.StatusFailed,
				Error:  updateErr.Error(),
			}
			a.reportJobResult(result)
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
		a.reportJobResult(result)
		slog.Info("set_host_report_interval accepted", "agent_id", a.cfg.AgentID, "job_id", job.JobID, "interval", nextInterval.String())
		return
	}

	// 上报任务开始事件
	_ = a.reportToBackend("job_event", map[string]any{
		"job_id":   job.JobID,
		"agent_id": a.cfg.AgentID,
		"action":   job.Action,
		"status":   protocol.StatusRunning,
		"error":    "",
		"ts":       time.Now().UTC().Format(time.RFC3339),
	})

	// 执行任务
	result, runErr := exec.Run(ctx, job)
	a.reportJobResult(result)

	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		slog.Warn("job execution finished with error", "agent_id", a.cfg.AgentID, "job_id", job.JobID, "err", runErr)
		return
	}
	slog.Info("job execution finished", "agent_id", a.cfg.AgentID, "job_id", job.JobID, "status", result.Status)
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

// buildCommandSubjects - 已弃用，使用RabbitMQ替代

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

// buildHostSnapshot 构建主机快照报告，用于上报给后端
func (a *App) buildHostSnapshot(exec *executor.Executor) map[string]any {
	job := protocol.Job{
		JobID:  fmt.Sprintf("periodic-host-info-%d", time.Now().Unix()),
		Type:   protocol.TaskTypeInventory,
		Action: "get_host_info",
	}

	result, runErr := exec.Run(context.Background(), job)
	payload := map[string]any{
		"agent_id":    a.cfg.AgentID,
		"action":      "get_host_info",
		"status":      result.Status,
		"result_data": result.Data,
		"error":       result.Error,
		"ts":          time.Now().UTC().Format(time.RFC3339),
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		payload["status"] = protocol.StatusFailed
		payload["error"] = runErr.Error()
	}
	return payload
}

// reportJobResult 上报任务执行结果给后端
func (a *App) reportJobResult(result protocol.JobResult) {
	payload := map[string]any{
		"job_id":        result.JobID,
		"status":        result.Status,
		"exit_code":     result.ExitCode,
		"stdout":        result.Stdout,
		"stderr":        result.Stderr,
		"result_data":   result.Data,
		"error_message": result.Error,
	}
	_ = a.reportToBackend("job_result", payload)
}

// reportToBackend 通过 RabbitMQ 上报数据给后端
func (a *App) reportToBackend(reportType string, payload map[string]any) error {
	if a.rabbitmqChannel == nil {
		err := fmt.Errorf("rabbitmq channel not initialized")
		slog.Error("rabbitmq channel not initialized", "type", reportType)
		return err
	}

	// 添加必要的字段
	payload["type"] = reportType
	payload["agent_id"] = a.cfg.AgentID
	if reportType == "heartbeat" || reportType == "agent_status" {
		payload["ts"] = time.Now().UTC().Format(time.RFC3339)
	}

	// 序列化为 JSON
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("marshal payload failed", "type", reportType, "err", err)
		return err
	}

	// 发送到 RabbitMQ agent.reports 队列
	err = a.rabbitmqChannel.PublishWithContext(
		context.Background(),
		"",              // exchange
		"agent.reports", // routing key (queue name)
		false,           // mandatory
		false,           // immediate
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
	if err != nil {
		slog.Warn("publish to rabbitmq failed", "type", reportType, "err", err)
		return err
	}

	slog.Info("report published to rabbitmq", "type", reportType, "agent_id", a.cfg.AgentID)
	return nil
}

func (a *App) gracefulShutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Best-effort offline signal: notify backend before stopping services.
	_ = a.reportToBackend("agent_status", map[string]any{
		"status": "offline",
		"reason": "shutdown",
	})
	slog.Info("offline status event published", "agent_id", a.cfg.AgentID)

	if err := a.stopHTTPServer(ctx); err != nil {
		return err
	}
	return nil
}
