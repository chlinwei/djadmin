package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/config"
	"github.com/chlinwei/djadmin/dj_agent/internal/executor"
	"github.com/chlinwei/djadmin/dj_agent/internal/grpcfile"
	"github.com/rabbitmq/amqp091-go"
)

type App struct {
	cfg                 config.Config
	rabbitmqChannel     *amqp091.Channel
	statusMu            sync.RWMutex
	startedAt           time.Time
	heartbeatInterval   time.Duration
	lastHeartbeatAt     time.Time
	lastHeartbeatStatus string
	lastHeartbeatError  string
	grpcConnected       bool
	isRunning           bool
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

	// 声明上报队列
	reportsQueue := "agent.reports"
	if _, err := channel.QueueDeclare(reportsQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare reports queue failed: %w", err)
	}

	// 启动背景服务
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 定时器
	heartbeatTicker := time.NewTicker(10 * time.Second)
	defer heartbeatTicker.Stop()

	// 启动统一 gRPC 通道客户端（agent 主动拨号连接 backend，断线自动重连）。
	// 该长连接承载文件传输、WebSSH 终端以及自动化任务同步执行，复用同一 exec 执行器。
	// 与 RabbitMQ 心跳/任务通道相互独立：即使这里连不上（backend 未起 gRPC 服务、
	// 或目标网络不通），也不影响心跳/任务/终端等既有功能正常运行。
	go grpcfile.Run(ctx, a.cfg.GRPCFileAddr, a.cfg.AgentID, exec, a.getRuntimeStatusData, a.setGRPCConnected)

	// RabbitMQ 心跳只有在 gRPC 通道建立后才代表 Agent 可执行，避免 API 重启后的短暂假在线。
	_ = a.reportToBackend("agent_status", map[string]any{
		"status": a.currentAgentStatus(),
		"reason": "startup_grpc_pending",
	})
	slog.Info("startup status event published", "agent_id", a.cfg.AgentID)

	for {
		select {
		case <-ctx.Done():
			slog.Info("shutdown signal received", "agent_id", a.cfg.AgentID)
			a.markStopped()

			shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
			defer cancel()

			return a.gracefulShutdown(shutdownCtx)
		case <-heartbeatTicker.C:
			a.markHeartbeatTick(time.Now())
			if err = a.reportToBackend("heartbeat", map[string]any{
				"agent_id":       a.cfg.AgentID,
				"grpc_connected": a.isGRPCConnected(),
				"ts":             time.Now().UTC().Format(time.RFC3339),
			}); err != nil {
				a.markHeartbeatResult("failed", err.Error())
			} else {
				a.markHeartbeatResult("success", "")
			}
		}
	}
}

func (a *App) setGRPCConnected(connected bool) {
	a.statusMu.Lock()
	changed := a.grpcConnected != connected
	a.grpcConnected = connected
	a.statusMu.Unlock()
	if changed {
		status := "offline"
		if connected {
			status = "online"
		}
		_ = a.reportToBackend("agent_status", map[string]any{
			"status":         status,
			"reason":         "grpc_connection_state_changed",
			"grpc_connected": connected,
		})
	}
}

func (a *App) isGRPCConnected() bool {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.grpcConnected
}

func (a *App) currentAgentStatus() string {
	if a.isGRPCConnected() {
		return "online"
	}
	return "offline"
}

func (a *App) markStarted() {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.startedAt = time.Now()
	a.isRunning = true
	a.lastHeartbeatAt = time.Time{}
	a.lastHeartbeatStatus = ""
	a.lastHeartbeatError = ""
	a.grpcConnected = false
}

func (a *App) markStopped() {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.isRunning = false
}

func (a *App) markHeartbeatTick(ts time.Time) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.lastHeartbeatAt = ts
}

func (a *App) markHeartbeatResult(status string, errText string) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.lastHeartbeatStatus = strings.TrimSpace(status)
	a.lastHeartbeatError = strings.TrimSpace(errText)
}

func (a *App) getRuntimeStatusData() map[string]any {
	a.statusMu.RLock()
	startedAt := a.startedAt
	isRunning := a.isRunning
	heartbeatInterval := a.heartbeatInterval
	lastHeartbeatAt := a.lastHeartbeatAt
	lastHeartbeatStatus := strings.TrimSpace(a.lastHeartbeatStatus)
	lastHeartbeatError := strings.TrimSpace(a.lastHeartbeatError)
	a.statusMu.RUnlock()

	now := time.Now()
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
			"listen_addr":  "-",
			"auth_enabled": false,
		},
		"grpc": map[string]any{
			"server_addr": a.cfg.GRPCFileAddr,
			"connected":   a.isGRPCConnected(),
		},
		"config": map[string]any{
			"max_workers":                           a.cfg.MaxWorkers,
			"shutdown_timeout_seconds":              int(a.cfg.ShutdownTimeout.Seconds()),
			"host_report_interval_fallback_raw":     a.cfg.HostReportIntervalRaw,
			"host_report_interval_fallback_seconds": int(a.cfg.HostReportInterval.Seconds()),
			"host_report_interval_current_seconds":  0,
		},
		"schedulers": map[string]any{
			"heartbeat": map[string]any{
				"enabled":          true,
				"interval_seconds": int(heartbeatInterval.Seconds()),
				"last_run_at":      toRFC3339(lastHeartbeatAt),
				"next_run_at":      resolveNextRunAt(lastHeartbeatAt, heartbeatInterval),
			},
		},
		"runtime": map[string]any{
			"mq_connected": a.rabbitmqChannel != nil,
		},
		"registered_tasks": registeredTasks,
	}
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
	return nil
}
