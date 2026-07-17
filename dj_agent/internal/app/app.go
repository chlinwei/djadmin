package app

import (
        "context"
        "log/slog"
        
        "os/signal"
        "syscall"
        "time"

        "github.com/chlinwei/djadmin/dj_agent/internal/config"
)

type App struct {
        cfg config.Config
}

func New(cfg config.Config) *App {
        return &App{cfg: cfg}
}

func (a *App) Run() error {
        slog.Info("app run begin", "agent_id", a.cfg.AgentID)

        ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
        defer stop()

        <-ctx.Done()
        slog.Info("shutdown signal received", "agent_id", a.cfg.AgentID)

        shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
        defer cancel()

        return a.gracefulShutdown(shutdownCtx)
}

func (a *App) gracefulShutdown(ctx context.Context) error {
        select {
        case <-time.After(2 * time.Second):
                return nil
        case <-ctx.Done():
                return ctx.Err()
        }
}