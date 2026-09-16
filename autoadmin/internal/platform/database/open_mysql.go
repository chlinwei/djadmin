//go:build !postgres

package database

import (
	"context"
	"database/sql"
	"fmt"
)

// Open 连接当前方言的数据库。默认（不带 build tag）是 MySQL；
// `-tags postgres` 换 open_postgres.go 里的 PostgreSQL 实现。
func Open(ctx context.Context, configuration Configuration) (*sql.DB, error) {
	if configuration.MySQLDSN == "" {
		return nil, fmt.Errorf("MYSQL_DSN is required for the default (MySQL) build")
	}
	return OpenMySQL(ctx, MySQLConfig{
		DSN:             configuration.MySQLDSN,
		MaxOpenConns:    configuration.MaxOpenConns,
		MaxIdleConns:    configuration.MaxIdleConns,
		ConnMaxLifetime: configuration.ConnMaxLifetime,
	})
}
