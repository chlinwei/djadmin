package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Environment                   string        `env:"APP_ENV" envDefault:"development"`
	HTTPAddress                   string        `env:"HTTP_ADDRESS" envDefault:":9000"`
	AgentGRPCAddress              string        `env:"AGENT_GRPC_ADDRESS" envDefault:":9001"`
	CORSOrigins                   []string      `env:"CORS_ALLOWED_ORIGINS" envDefault:"*" envSeparator:","`
	ShutdownTimeout               time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`
	MySQLDSN                      string        `env:"MYSQL_DSN"`
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
	MigrationSource               string        `env:"MIGRATION_SOURCE_URL" envDefault:"file://db/migrations"`
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
	if command != "migrate" && configuration.MySQLDSN == "" {
		return fmt.Errorf("MYSQL_DSN is required for %s", command)
	}
	if command == "api" && configuration.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required for api")
	}
	if (command == "api" || command == "scheduler" || command == "worker") && configuration.RabbitMQURL == "" {
		return fmt.Errorf("RABBITMQ_URL is required for %s", command)
	}
	return nil
}
