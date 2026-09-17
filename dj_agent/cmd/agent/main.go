package main

import (
	"log/slog"
	"os"

	"github.com/chlinwei/djadmin/dj_agent/internal/app"
	"github.com/chlinwei/djadmin/dj_agent/internal/buildinfo"
	"github.com/chlinwei/djadmin/dj_agent/internal/config"
	"github.com/chlinwei/djadmin/dj_agent/internal/logger"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		println("dj-agent " + buildinfo.Version)
		return
	}
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
		"version", buildinfo.Version,
		"pid", os.Getpid(),
		"instance_name", cfg.InstanceName,
		"log_level", cfg.LogLevel,
		"max_workers", cfg.MaxWorkers,
		"shutdown_timeout", cfg.ShutdownTimeout.String(),
		"grpc_file_addr", cfg.GRPCFileAddr,
	)

	if err := app.New(cfg).Run(); err != nil {
		return err
	}

	slog.Info("dj_agent stopped", "instance_name", cfg.InstanceName)
	return nil
}
