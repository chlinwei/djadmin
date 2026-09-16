//go:build !postgres

// 迁移驱动按构建标签选择：默认构建（MySQL）只注册 mysql 驱动，`-tags postgres` 只注册 pgx5。
// 两个驱动各自注册自己认识的 URL scheme，同时在同一个二进制里注册会让 migrate.New 在
// 解析 DSN 时按 scheme 分派（但如果只注册了 mysql、却给了 postgres:// 的 URL，migrate 会直接
// 报 "unknown database scheme"）——这正是 P1-7 要修的一半。
package migration

import (
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
)
