package config

import (
    "testing"
    "time"
)

func TestValidate_OK(t *testing.T) {
    cfg := Config{
        AgentID:         "agent-001",
        LogLevel:        "info",
        MaxWorkers:      2,
        ShutdownTimeout: 5 * time.Second,
    }
    if err := cfg.Validate(); err != nil {
        t.Fatalf("expected nil error, got: %v", err)
    }
}

func TestValidate_BadLogLevel(t *testing.T) {
    cfg := Config{
        AgentID:         "agent-001",
        LogLevel:        "verbose",
        MaxWorkers:      2,
        ShutdownTimeout: 5 * time.Second,
    }
    if err := cfg.Validate(); err == nil {
        t.Fatalf("expected error, got nil")
    }
}

func TestLoadFromEnv_Override(t *testing.T) {
    t.Setenv("DJ_AGENT_ID", "agent-from-env")
    t.Setenv("DJ_AGENT_LOG_LEVEL", "debug")
    t.Setenv("DJ_AGENT_MAX_WORKERS", "7")
    t.Setenv("DJ_AGENT_SHUTDOWN_TIMEOUT", "3s")

    cfg, err := LoadFromEnv()
    if err != nil {
        t.Fatalf("LoadFromEnv failed: %v", err)
    }

    if cfg.AgentID != "agent-from-env" {
        t.Fatalf("AgentID mismatch: %s", cfg.AgentID)
    }
    if cfg.LogLevel != "debug" {
        t.Fatalf("LogLevel mismatch: %s", cfg.LogLevel)
    }
    if cfg.MaxWorkers != 7 {
        t.Fatalf("MaxWorkers mismatch: %d", cfg.MaxWorkers)
    }
    if cfg.ShutdownTimeout != 3*time.Second {
        t.Fatalf("ShutdownTimeout mismatch: %s", cfg.ShutdownTimeout)
    }
}
