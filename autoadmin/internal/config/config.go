package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Environment      string `env:"APP_ENV" envDefault:"development"`
	HTTPAddress      string `env:"HTTP_ADDRESS" envDefault:":9000"`
	AgentGRPCAddress string `env:"AGENT_GRPC_ADDRESS" envDefault:":9001"`
	// AgentGRPCAuthMode: open=放行所有 agent 连接（过渡期，后续切 mTLS）；token=校验 sys_agent_token。
	AgentGRPCAuthMode string        `env:"AGENT_GRPC_AUTH_MODE" envDefault:"open"`
	CORSOrigins       []string      `env:"CORS_ALLOWED_ORIGINS" envDefault:"*" envSeparator:","`
	ShutdownTimeout   time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`
	MySQLDSN          string        `env:"MYSQL_DSN"`
	// PostgresDSN 只在 -tags postgres 的构建里被使用（见 platform/database/open_postgres.go）。
	PostgresDSN                   string        `env:"POSTGRES_DSN"`
	MySQLMaxOpen                  int           `env:"MYSQL_MAX_OPEN_CONNS" envDefault:"30"`
	MySQLMaxIdle                  int           `env:"MYSQL_MAX_IDLE_CONNS" envDefault:"10"`
	MySQLMaxLife                  time.Duration `env:"MYSQL_CONN_MAX_LIFETIME" envDefault:"30m"`
	RabbitMQURL                   string        `env:"RABBITMQ_URL"`
	WorkerName                    string        `env:"WORKER_NAME" envDefault:"autoadmin-worker"`
	WorkerPrefetch                int           `env:"WORKER_PREFETCH" envDefault:"4"`
	JWTSecret                     string        `env:"JWT_SECRET"`
	JWTExpiration                 time.Duration `env:"JWT_EXPIRATION" envDefault:"24h"`
	AssetsCredentialEncryptionKey string        `env:"ASSETS_CREDENTIAL_ENCRYPTION_KEY"`
	MigrationDBURL                string        `env:"MIGRATION_DATABASE_URL"`
	MigrationSource               string        `env:"MIGRATION_SOURCE_URL" envDefault:"file://db/migrations/mysql"`
}

func Load() (Config, error) {
	configuration, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("parse environment: %w", err)
	}
	return configuration, nil
}

func (configuration Config) Validate(command string) error {
	if command == "migrate" && configuration.MigrationDBURL == "" {
		return fmt.Errorf("MIGRATION_DATABASE_URL is required for migrate")
	}
	// 具体用哪个 DSN 由构建期方言决定：默认构建要 MYSQL_DSN，-tags postgres 要 POSTGRES_DSN。
	// 这里只保证至少配了一个，缺哪个由 dialect 的 Open 报错并指明变量名。
	if command != "migrate" && configuration.MySQLDSN == "" && configuration.PostgresDSN == "" {
		return fmt.Errorf("MYSQL_DSN or POSTGRES_DSN is required for %s", command)
	}
	if command == "api" && configuration.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required for api")
	}
	if (command == "api" || command == "scheduler" || command == "worker") && configuration.RabbitMQURL == "" {
		return fmt.Errorf("RABBITMQ_URL is required for %s", command)
	}
	return nil
}
