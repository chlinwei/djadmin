package app

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/config"
	"github.com/chlinwei/djadmin/dj_agent/internal/executor"
	"github.com/chlinwei/djadmin/dj_agent/internal/grpcfile"
)

type App struct {
	cfg           config.Config
	statusMu      sync.RWMutex
	startedAt     time.Time
	grpcConnected bool
	isRunning     bool
}

func New(cfg config.Config) *App {
	return &App{cfg: cfg}
}

func (a *App) Run() error {
	slog.Info("app run begin", "agent_id", a.cfg.AgentID)
	a.markStarted()
	exec := executor.New(0)

	// 启动背景服务
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 启动统一 gRPC 通道客户端（agent 主动拨号连接 backend，断线自动重连）。
	// 该长连接承载文件传输、WebSSH 终端以及自动化任务同步执行，复用同一 exec 执行器。
	// backend 未启动或网络中断时，客户端会持续重连，不结束 Agent 进程。
	go grpcfile.Run(ctx, a.cfg.GRPCFileAddr, a.cfg.AgentID, a.cfg.BackendToken, exec, a.getRuntimeStatusData, a.setGRPCConnected)

	for {
		select {
		case <-ctx.Done():
			slog.Warn("shutdown signal received; agent will exit", "agent_id", a.cfg.AgentID, "signal", "SIGTERM or SIGINT", "grpc_connected", a.isGRPCConnected())
			a.markStopped()

			shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
			defer cancel()

			if err := a.gracefulShutdown(shutdownCtx); err != nil {
				slog.Error("graceful shutdown failed", "agent_id", a.cfg.AgentID, "err", err)
				return err
			}
			slog.Warn("agent stopped after shutdown signal", "agent_id", a.cfg.AgentID)
			return nil
		}
	}
}

func (a *App) setGRPCConnected(connected bool) {
	a.statusMu.Lock()
	changed := a.grpcConnected != connected
	a.grpcConnected = connected
	a.statusMu.Unlock()
	if changed {
		slog.Info("grpc connection state changed", "agent_id", a.cfg.AgentID, "connected", connected)
	}
}

func (a *App) isGRPCConnected() bool {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.grpcConnected
}

func (a *App) markStarted() {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.startedAt = time.Now()
	a.isRunning = true
	a.grpcConnected = false
}

func (a *App) markStopped() {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.isRunning = false
}

func (a *App) getRuntimeStatusData() map[string]any {
	a.statusMu.RLock()
	startedAt := a.startedAt
	isRunning := a.isRunning
	a.statusMu.RUnlock()

	now := time.Now()
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
		"schedulers":       map[string]any{},
		"runtime":          map[string]any{},
		"registered_tasks": []map[string]any{},
	}
}

func (a *App) gracefulShutdown(ctx context.Context) error {
	return ctx.Err()
}
