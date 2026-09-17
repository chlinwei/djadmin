package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	InstanceName    string
	LogLevel        string
	MaxWorkers      int
	ShutdownTimeout time.Duration
	BackendToken    string
	GRPCFileAddr    string
}

func LoadFromEnv() (Config, error) {
	// 实例名（assets_host.instance_name）是主机的全局唯一标识：agent 握手时上报此值，
	// backend 按 instance_name 匹配 assets_host 中的主机行。该值必须与前端创建主机时
	// 填的实例名一致，因此没有默认值——缺失即启动失败，避免静默用一个无意义的标识。
	cfg := Config{
		InstanceName:    strings.TrimSpace(os.Getenv("DJ_AGENT_INSTANCE_NAME")),
		LogLevel:        strings.ToLower(getEnv("DJ_AGENT_LOG_LEVEL", "info")),
		MaxWorkers:      3,
		ShutdownTimeout: 5 * time.Second,
		BackendToken:    strings.TrimSpace(os.Getenv("DJ_AGENT_BACKEND_TOKEN")),
		// Agent 主动拨号连接 backend，不要求 backend 主动访问目标主机。
		GRPCFileAddr: strings.TrimSpace(getEnv("DJ_AGENT_GRPC_FILE_ADDR", "127.0.0.1:9001")),
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
	if c.InstanceName == "" {
		return fmt.Errorf("DJ_AGENT_INSTANCE_NAME is required（须与 assets_host.instance_name 一致）")
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
