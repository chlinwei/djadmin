//go:build postgres

package database

import (
	"context"
	"database/sql"
	"fmt"

	// pgx 的 database/sql 适配器：注册 "pgx" 驱动，让应用侧继续用 *sql.DB。
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Open 连接 PostgreSQL（`-tags postgres` 时替代 open_mysql.go 的实现）。
//
// DSN 用 pgx 的 URL 或 key=value 形式，例如：
//
//	postgres://user:pass@host:5432/dbname?sslmode=disable
//
// 时间类型：MySQL 侧通过对 DSN 强制 loc=UTC 统一到 UTC，这里要求 DSN 里带
// TimeZone=UTC（或服务端 timezone=UTC），否则 timestamp 的时区语义与 MySQL 侧不一致。
func Open(ctx context.Context, configuration Configuration) (*sql.DB, error) {
	if configuration.PostgresDSN == "" {
		return nil, fmt.Errorf("POSTGRES_DSN is required for the postgres build")
	}
	connection, err := sql.Open("pgx", configuration.PostgresDSN)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	connection.SetMaxOpenConns(configuration.MaxOpenConns)
	connection.SetMaxIdleConns(configuration.MaxIdleConns)
	connection.SetConnMaxLifetime(configuration.ConnMaxLifetime)
	if err := connection.PingContext(ctx); err != nil {
		connection.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return connection, nil
}
