//go:build postgres

// `-tags postgres` 下用 pgx v5 的迁移驱动（与连接层 internal/platform/database/open_postgres.go
// 用同一个 driver 家族）。注意 URL scheme 是 **pgx5://** 而不是 postgres://：
// golang-migrate 按 scheme 分派驱动（database/pgx/v5 注册的名字就是 "pgx5"）。
// 所以 PG 变体的 MIGRATION_DATABASE_URL 形如：
//
//	pgx5://user:pass@host:5432/dbname?sslmode=disable
package migration

import (
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
)
