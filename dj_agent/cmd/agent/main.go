package main

import (
	"log/slog"
	"os"

	"github.com/chlinwei/djadmin/dj_agent/internal/app"
	"github.com/chlinwei/djadmin/dj_agent/internal/config"
	"github.com/chlinwei/djadmin/dj_agent/internal/logger"
)

func main() {
	if err := run(); err != nil {
		slog.Error("dj_agent exit with error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	logger.Init(cfg.LogLevel)
	slog.Info("dj_agent starting",
		"version", "dev",
		"pid", os.Getpid(),
		"agent_id", cfg.AgentID,
		"log_level", cfg.LogLevel,
		"max_workers", cfg.MaxWorkers,
		"shutdown_timeout", cfg.ShutdownTimeout.String(),
		"backend_base_url", cfg.BackendBaseURL,
		"grpc_file_addr", cfg.GRPCFileAddr,
		"host_report_interval_fallback", cfg.HostReportInterval.String(),
	)

	if err := app.New(cfg).Run(); err != nil {
		return err
	}

	slog.Info("dj_agent stopped", "agent_id", cfg.AgentID)
	return nil
}
