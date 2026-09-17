package database_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"autoadmin/internal/platform/database"
)

// TestSchemaMatchesRealDatabase 是本仓库"db/schema 与真实库一致性"的守卫（P3-4）。
//
// 背景：`monitor_alert_route` 曾被迁移删掉但 db/schema 没同步，sqlc 静默生成了
// 打不存在表的查询（SQL_DESIGN §6.1）；`inspection_target_execution` 的列与真库
// 也有过漂移（P5 陷阱 24）。这类漂移只有真跑才暴露，sqlc 不做执行。
//
// 做法：解析 `db/schema/<dialect>` 的表与列，逐项断言真实库里存在。**只检查
// "schema 有而库里没有"这个方向**：真库比模型多出的表是 Django 框架记账表
// （auth_* / django_* / schema_migrations），按设计不建模（SQL_DESIGN §6.2）。
//
// 需要真库，未设 `SCHEMA_GUARD_DSN` 时跳过：
//
//	SCHEMA_GUARD_DSN='root:pwd@tcp(host:3306)/djadmin?parseTime=true&loc=UTC' go test ./internal/platform/database/ -run SchemaMatches
//	SCHEMA_GUARD_DSN='postgres://user@host:5432/db?sslmode=disable&TimeZone=UTC' go test -tags postgres ./internal/platform/database/ -run SchemaMatches
func TestSchemaMatchesRealDatabase(t *testing.T) {
	dsn := os.Getenv("SCHEMA_GUARD_DSN")
	if dsn == "" {
		t.Skip("SCHEMA_GUARD_DSN 未设置：跳过 schema 与真库一致性检查（说明见本函数注释）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	connection, err := database.Open(ctx, database.Configuration{
		MySQLDSN: dsn, PostgresDSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("连接数据库失败：%v", err)
	}
	defer connection.Close()

	live, err := liveSchemaColumns(ctx, connection)
	if err != nil {
		t.Fatalf("读取 information_schema 失败：%v", err)
	}

	root := autoadminRoot(t)
	modeled, err := parseSchemaColumns(filepath.Join(root, "db", "schema", schemaDialect()))
	if err != nil {
		t.Fatalf("解析 db/schema/%s 失败：%v", schemaDialect(), err)
	}
	if len(live) == 0 || len(modeled) == 0 {
		t.Fatalf("解析结果为空（真库 %d 表 / schema %d 表），检查 DSN 是否指向正确的库",
			len(live), len(modeled))
	}

	tables := make([]string, 0, len(modeled))
	for table := range modeled {
		tables = append(tables, table)
	}
	sort.Strings(tables)

	for _, table := range tables {
		liveColumns, ok := live[table]
		if !ok {
			t.Errorf("db/schema 有表 %s，但真实库里不存在（幽灵表，见 SQL_DESIGN §6.1）", table)
			continue
		}
		var missing []string
		for _, column := range modeled[table] {
			if !liveColumns[column.Name] {
				missing = append(missing, column.Name)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Errorf("表 %s 在 db/schema 里有列 %v，但真实库里不存在（schema 漂移）", table, missing)
		}
	}
	t.Logf("已比对 db/schema/%s 的 %d 张表与真库", schemaDialect(), len(modeled))
}

// scanSchemaRows 把 (table, column) 结果集整理成 表名 → 列名集合。
func scanSchemaRows(rows *sql.Rows) (map[string]map[string]bool, error) {
	tables := make(map[string]map[string]bool)
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return nil, err
		}
		if tables[table] == nil {
			tables[table] = make(map[string]bool)
		}
		tables[table][column] = true
	}
	return tables, rows.Err()
}

// liveSchemaColumns 由方言文件实现，返回 表名 → 列名集合。
var liveSchemaColumns func(ctx context.Context, db *sql.DB) (map[string]map[string]bool, error)

// schemaDialect 由方言文件实现，返回当前构建对应的 schema 目录名。
var schemaDialect func() string
