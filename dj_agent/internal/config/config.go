package config

import (
    "fmt"
    "os"
    "strconv"
    "strings"
    "time"
)

type Config struct {
    AgentID         string
    LogLevel        string
    MaxWorkers      int
    ShutdownTimeout time.Duration
}

func LoadFromEnv() (Config, error) {
    cfg := Config{
        AgentID:         getEnv("DJ_AGENT_ID", "agent-dev"),
        LogLevel:        strings.ToLower(getEnv("DJ_AGENT_LOG_LEVEL", "info")),
        MaxWorkers:      3,
        ShutdownTimeout: 5 * time.Second,
    }

    if v := os.Getenv("DJ_AGENT_MAX_WORKERS"); v != "" {
        n, err := strconv.Atoi(v)
        if err != nil {
            return Config{}, fmt.Errorf("invalid DJ_AGENT_MAX_WORKERS: %w", err)
        }
        cfg.MaxWorkers = n
    }

    if v := os.Getenv("DJ_AGENT_SHUTDOWN_TIMEOUT"); v != "" {
        d, err := time.ParseDuration(v)
        if err != nil {
            return Config{}, fmt.Errorf("invalid DJ_AGENT_SHUTDOWN_TIMEOUT: %w", err)
        }
        cfg.ShutdownTimeout = d
    }

    if err := cfg.Validate(); err != nil {
        return Config{}, err
    }
    return cfg, nil
}

func (c Config) Validate() error {
    if strings.TrimSpace(c.AgentID) == "" {
        return fmt.Errorf("agent_id is required")
    }
    if c.MaxWorkers <= 0 {
        return fmt.Errorf("max_workers must be > 0")
    }
    if c.ShutdownTimeout <= 0 {
        return fmt.Errorf("shutdown_timeout must be > 0")
    }
    switch c.LogLevel {
    case "debug", "info", "warn", "error":
    default:
        return fmt.Errorf("log_level must be one of debug/info/warn/error")
    }
    return nil
}

func getEnv(key, def string) string {
    v := strings.TrimSpace(os.Getenv(key))
    if v == "" {
        return def
    }
    return v
}