package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/platform/database"
	db "autoadmin/internal/platform/database/generated"
)

// TestInsertQueriesReturnLastInsertIDAgainstRealDatabase 在**真库**上验证"插入并取回自增主键"这条路。
//
// 为什么单列一个用例：MySQL 靠驱动的 `LastInsertId()` 取主键，PostgreSQL 的驱动层不实现它——
// pgx 对普通 Exec 返回 `driver.RowsAffected`，`LastInsertId()` 恒报
// `LastInsertId is not supported by this driver`。这类差异**两个 tag 下都能编译、mock 用例也全绿**，
// 只有真库会炸（2026-09-16 实测：baseline 的 27 步冒烟失败、其余模块的新建接口全部不可用）。
// 派生脚本因此把 INSERT 的 `:execlastid` / `:execresult` 都改写成 PG 的 `:one` + `RETURNING id`，
// 并在门面里把返回的 id 包成 `sql.Result`（见 dialect_postgres_adapters.go）。
//
// 这里覆盖各域的代表性插入，断言的是**调用点真正依赖的契约**：`LastInsertId()` 返回真实主键、
// `RowsAffected()==1`。全程一个事务，结束回滚，不留数据。
//
// 用法（DB 里要有 db/schema 对应的表）：
//
//	DB_SMOKE_DSN='root:pwd@tcp(host:3306)/djadmin?parseTime=true&loc=UTC' go test ./internal/platform/database/ -run Insert -v
//	DB_SMOKE_DSN='postgres://user@host:5432/db?sslmode=disable&TimeZone=UTC' go test -tags postgres ./internal/platform/database/ -run Insert -v
func TestInsertQueriesReturnLastInsertIDAgainstRealDatabase(t *testing.T) {
	dsn := os.Getenv("DB_SMOKE_DSN")
	if dsn == "" {
		t.Skip("DB_SMOKE_DSN 未设置：跳过真库插入冒烟（说明见本函数注释）")
	}
	ctx := context.Background()
	connection, err := database.Open(ctx, database.Configuration{
		MySQLDSN: dsn, PostgresDSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("connect(%s): %v", redactDSN(dsn), err)
	}
	defer connection.Close()
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()
	queries := db.New(tx)

	now := time.Now().UTC()
	suffix := now.Format("150405.000000")
	nullString := func(value string) sql.NullString { return sql.NullString{String: value, Valid: true} }
	nullTime := func(value time.Time) sql.NullTime { return sql.NullTime{Time: value, Valid: true} }

	// 顺序即依赖顺序（子表要挂到刚建的父行上）；每条都必须给出真实自增主键——
	// 这正是 PG 侧 LastInsertId 不可用时会退化成 0/报错的地方。
	var projectID, applicationID int64
	inserts := []struct {
		name string
		run  func() (sql.Result, error)
	}{
		{"CreateProject", func() (sql.Result, error) {
			result, err := queries.CreateProject(ctx, db.CreateProjectParams{CreateTime: now, UpdateTime: now,
				Name: "smoke-" + suffix, Code: "smoke-" + suffix, Owner: "smoke", Enabled: true})
			if err == nil {
				projectID, err = result.LastInsertId()
			}
			return result, err
		}},
		{"CreateBusinessSystem", func() (sql.Result, error) {
			return queries.CreateBusinessSystem(ctx, db.CreateBusinessSystemParams{CreateTime: now, UpdateTime: now,
				Name: "smoke-" + suffix, Code: "smoke-" + suffix, Owner: "smoke", Enabled: true,
				ProjectID: sql.NullInt64{Int64: projectID, Valid: true}})
		}},
		{"CreateBusinessEnvironment", func() (sql.Result, error) {
			return queries.CreateBusinessEnvironment(ctx, db.CreateBusinessEnvironmentParams{CreateTime: now, UpdateTime: now,
				Name: "smoke-" + suffix, Code: "smoke-" + suffix, Order: 1, Owner: "smoke", Enabled: true})
		}},
		{"CreateCredential", func() (sql.Result, error) {
			return queries.CreateCredential(ctx, db.CreateCredentialParams{CreateTime: now, UpdateTime: now,
				Name: nullString("smoke-" + suffix), AuthType: 1, Username: "smoke", Port: 22})
		}},
		{"CreateHostGroup", func() (sql.Result, error) {
			return queries.CreateHostGroup(ctx, db.CreateHostGroupParams{CreateTime: now, UpdateTime: now,
				Name: "smoke-" + suffix})
		}},
		{"CreateHost", func() (sql.Result, error) {
			return queries.CreateHost(ctx, db.CreateHostParams{CreateTime: now, UpdateTime: now, Status: "active",
				CollectStatus: "pending", InstanceName: nullString("smoke-" + suffix), Ip: nullString("10.255.255.1")})
		}},
		{"CreateApplication", func() (sql.Result, error) {
			result, err := queries.CreateApplication(ctx, db.CreateApplicationParams{CreateTime: now, UpdateTime: now,
				Name: "smoke-" + suffix, Category: "web", Code: "smoke-" + suffix, Vendor: "smoke", Enabled: true})
			if err == nil {
				applicationID, err = result.LastInsertId()
			}
			return result, err
		}},
		{"CreateApplicationVersion", func() (sql.Result, error) {
			return queries.CreateApplicationVersion(ctx, db.CreateApplicationVersionParams{CreateTime: now, UpdateTime: now,
				Version: "1.0.0", Enabled: true, ApplicationID: applicationID})
		}},
		{"CreateClusterProfile", func() (sql.Result, error) {
			return queries.CreateClusterProfile(ctx, db.CreateClusterProfileParams{CreateTime: now, UpdateTime: now,
				Name: "smoke-" + suffix, Code: "smoke-" + suffix, ProfileType: "standalone", Enabled: true,
				ApplicationID: sql.NullInt64{Int64: applicationID, Valid: true}, ClusterType: "single"})
		}},
		{"CreateUser", func() (sql.Result, error) {
			return queries.CreateUser(ctx, db.CreateUserParams{Username: "smoke-" + suffix, Password: "x",
				Status: 1, Timezone: "Asia/Shanghai", CreateTime: nullTime(now), UpdateTime: nullTime(now)})
		}},
		{"CreateRole", func() (sql.Result, error) {
			return queries.CreateRole(ctx, db.CreateRoleParams{Name: nullString("smoke-" + suffix),
				Code: nullString("smoke-" + suffix), CreateTime: nullTime(now), UpdateTime: nullTime(now)})
		}},
		{"CreateMenu", func() (sql.Result, error) {
			return queries.CreateMenu(ctx, db.CreateMenuParams{Name: "smoke-" + suffix, Location: 1,
				CreateTime: nullTime(now), UpdateTime: nullTime(now)})
		}},
		{"CreateAPIToken", func() (sql.Result, error) {
			return queries.CreateAPIToken(ctx, db.CreateAPITokenParams{AgentID: "smoke-" + suffix, TokenHash: "hash",
				IsActive: true, BindMode: "strict", CreateTime: now, UpdateTime: now})
		}},
		{"CreateConfig", func() (sql.Result, error) {
			return queries.CreateConfig(ctx, db.CreateConfigParams{Name: "smoke-" + suffix, Key: "smoke." + suffix,
				Value: "1", ValueType: "string", CreateTime: now, UpdateTime: now})
		}},
	}
	for _, item := range inserts {
		t.Run(item.name, func(t *testing.T) {
			result, err := item.run()
			if err != nil {
				t.Fatalf("insert: %v", err)
			}
			id, err := result.LastInsertId()
			if err != nil {
				t.Fatalf("LastInsertId: %v（PG 侧说明 INSERT 没有派生为 :one + RETURNING）", err)
			}
			if id <= 0 {
				t.Fatalf("LastInsertId = %d，应为真实自增主键", id)
			}
			affected, err := result.RowsAffected()
			if err != nil || affected != 1 {
				t.Fatalf("RowsAffected = %d, %v，应为 1", affected, err)
			}
		})
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
}

// redactDSN 只保留主机与库名，避免把口令写进测试日志。
func redactDSN(dsn string) string {
	if at := strings.LastIndex(dsn, "@"); at >= 0 {
		return fmt.Sprintf("***%s", dsn[at:])
	}
	return dsn
}
