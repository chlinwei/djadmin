package app

import (
	"context"
	"log/slog"
	"os/signal"
	"sync"
	"syscall"

	"github.com/chlinwei/djadmin/dj_agent/internal/config"
	"github.com/chlinwei/djadmin/dj_agent/internal/executor"
	"github.com/chlinwei/djadmin/dj_agent/internal/grpcfile"
)

type App struct {
	cfg           config.Config
	statusMu      sync.RWMutex
	grpcConnected bool
}

func New(cfg config.Config) *App {
	return &App{cfg: cfg}
}

func (a *App) Run() error {
	slog.Info("app run begin", "instance_name", a.cfg.InstanceName)
	exec := executor.New(0)

	// 启动背景服务
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 启动统一 gRPC 通道客户端（agent 主动拨号连接 backend，断线自动重连）。
	// 该长连接承载文件传输、WebSSH 终端以及自动化任务同步执行，复用同一 exec 执行器。
	// backend 未启动或网络中断时，客户端会持续重连，不结束 Agent 进程。
	go grpcfile.Run(ctx, a.cfg.GRPCFileAddr, a.cfg.InstanceName, a.cfg.BackendToken, exec, a.setGRPCConnected)

	for {
		select {
		case <-ctx.Done():
			slog.Warn("shutdown signal received; agent will exit", "instance_name", a.cfg.InstanceName, "signal", "SIGTERM or SIGINT", "grpc_connected", a.isGRPCConnected())

			shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
			defer cancel()

			if err := a.gracefulShutdown(shutdownCtx); err != nil {
				slog.Error("graceful shutdown failed", "instance_name", a.cfg.InstanceName, "err", err)
				return err
			}
			slog.Warn("agent stopped after shutdown signal", "instance_name", a.cfg.InstanceName)
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
		slog.Info("grpc connection state changed", "instance_name", a.cfg.InstanceName, "connected", connected)
	}
}

func (a *App) isGRPCConnected() bool {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.grpcConnected
}

func (a *App) gracefulShutdown(ctx context.Context) error {
	return ctx.Err()
}
