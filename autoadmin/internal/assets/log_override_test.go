package assets

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// 按行写日志覆盖值（采集开关 / 保留档位）的两个必须守住的语义：
//  1. 一次内联改动**只能**影响这一条 (服务 × 日志定义)，不能碰同服务其他日志的覆盖值
//     ——整表替换那条路（SaveApplicationService 的 log_settings）没有这个保证，所以内联操作
//     必须走这里（见 log_setting.go 的注释与架构文档 §9.5）；
//  2. **不能碰 format_verified_\***：改采集开关/档位不改日志格式、也不进认证指纹，
//     认证状态必须原样保留——否则用户一改开关就要重新认证一遍。

// 守卫：按行 upsert 的 SQL 里不许出现认证四列。
//
// 这条比"写一条断言"更值得放守卫的原因是它防的是**将来**的改动：给 UPDATE 分支顺手补一个
// format_verified_fingerprint 是很容易发生的事（"既然是同一行的列，一起更新吧"），
// 而后果是静默把认证结果抹掉（指纹被覆盖成旧值 → format_state 变成 needs_recheck）。
func TestUpsertServiceLogOverrideKeepsFormatVerification(t *testing.T) {
	for _, path := range []string{
		"db/queries/mysql/assets.sql",
		"db/queries/postgres/assets.sql",
	} {
		content, err := readRepositoryFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		statement := namedStatement(content, "UpsertServiceLogOverride")
		if statement == "" {
			t.Fatalf("%s 里找不到 UpsertServiceLogOverride 语句", path)
		}
		if strings.Contains(statement, "format_verified") {
			t.Fatalf("%s 的 UpsertServiceLogOverride 触碰了 format_verified_* 列；"+
				"改采集开关/档位不得影响认证结果：\n%s", path, statement)
		}
		// 覆盖列必须只有这两项：多写一列就可能顺手polish掉别的语义。
		for _, column := range []string{"collection_enabled", "retention_tier_id"} {
			if !strings.Contains(statement, column) {
				t.Fatalf("%s 的 UpsertServiceLogOverride 少了覆盖列 %s", path, column)
			}
		}
	}
}

func overrideFixtureRows(stored sql.NullString, collectionEnabled *bool, tier sql.NullInt64) *sqlmock.Rows {
	rows := sqlmock.NewRows(serviceTemplateLogColumns())
	rows.AddRow(
		int64(24), "error.log", "${APP_HOME}/nginx/logs/error.log", sql.NullInt64{Int64: 7, Valid: true},
		"nginx 规则", testRuleUpdatedAt, int64(3), tier,
		collectionEnabled, sql.NullInt64{},
		sql.NullTime{}, stored, sql.NullString{}, sql.NullString{},
		"nginx", "yilake", sql.NullString{String: "poc", Valid: true}, "tib", []byte("{}"), "std",
	)
	return rows
}

func TestSaveServiceLogOverrideWritesSingleRowAndKeepsVerification(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))

	// 1) 校验目标：读该服务的模板日志定义
	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(overrideFixtureRows(sql.NullString{}, nil, sql.NullInt64{}))
	// 2) 按行 upsert（必须是这一条语句：整表替换会先 DELETE）
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO assets_application_service_log_setting")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), int64(24), sqlmock.AnyArg(), int64(15)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 3) 回读整行（覆盖值与认证状态都由后端算）
	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(overrideFixtureRows(
			sql.NullString{String: currentLogFingerprint(), Valid: true}, boolPtr(false), sql.NullInt64{Int64: 3, Valid: true}))

	disabled := false
	item, err := service.SaveServiceLogOverride(context.Background(), 15, ServiceLogOverrideInput{
		LogDefinition: 24, CollectionEnabled: &disabled, RetentionTier: int64Ptr(3),
	})
	if err != nil {
		t.Fatalf("save override: %v", err)
	}
	// 回读的认证状态保持 verified：改开关/档位不该让认证失效。
	if item.FormatState != formatStateVerified {
		t.Fatalf("format_state = %q, want %q（改采集开关/档位不应影响认证）", item.FormatState, formatStateVerified)
	}
	if item.CollectionEnabled == nil || *item.CollectionEnabled {
		t.Fatalf("回读的采集开关应为 false，得到 %+v", item.CollectionEnabled)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

// 目标日志定义不属于该服务当前模板时拒绝：否则就是往别的服务的行上写覆盖值。
func TestSaveServiceLogOverrideRejectsForeignLogDefinition(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))

	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(overrideFixtureRows(sql.NullString{}, nil, sql.NullInt64{}))

	// 模板里只有 24，这里传 999：没有任何写库预期，写了会被 ExpectationsWereMet 抓到。
	if _, err = service.SaveServiceLogOverride(context.Background(), 15, ServiceLogOverrideInput{LogDefinition: 999}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不属于该模板的日志定义应报 ErrNotFound，得到 %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("不该发生写入: %v", err)
	}
}

// 参数非法在读库之前就拒绝（不浪费查询，也不给"写一半"的机会）。
func TestSaveServiceLogOverrideRejectsInvalidArguments(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))

	if _, err = service.SaveServiceLogOverride(context.Background(), 15, ServiceLogOverrideInput{LogDefinition: 0}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("缺少日志定义应报 ErrInvalid，得到 %v", err)
	}
	if _, err = service.SaveServiceLogOverride(context.Background(), 0, ServiceLogOverrideInput{LogDefinition: 24}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("缺少服务 id 应报 ErrInvalid，得到 %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("不该发生任何查询: %v", err)
	}
}

func int64Ptr(value int64) *int64 { return &value }

// readRepositoryFile 读模块内的文件（按 go.mod 定位根目录，与 dropped_column_guard_test.go 同一套）。
func readRepositoryFile(relativePath string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(findRepositoryRoot(), relativePath))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// namedStatement 截出 `-- name: X` 到下一个 `-- name:` 之间的 SQL 文本
// （sqlc 的语句边界就是它，守卫只需看这一段）。
func namedStatement(content, name string) string {
	marker := "-- name: " + name
	start := strings.Index(content, marker)
	if start < 0 {
		return ""
	}
	rest := content[start+len(marker):]
	if next := strings.Index(rest, "-- name: "); next >= 0 {
		rest = rest[:next]
	}
	return rest
}

// findRepositoryRoot 从测试工作目录向上找 go.mod（本模块根），用于读取模块内文件。
func findRepositoryRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for i := 0; i < 10; i++ {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

// 服务级采集总开关：单独写入（原先只能随整份服务表单提交），并且服务不存在时**必须报错**——
// RowsAffected 为 0 却回成功，会让用户以为"关掉了"，实际什么都没发生。
func TestSetServiceLogCollectionWritesAndDetectsMissingService(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))

	mock.ExpectExec(regexp.QuoteMeta("UPDATE assets_application_service")).
		WithArgs(sqlmock.AnyArg(), false, int64(15)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	enabled, err := service.SetServiceLogCollection(context.Background(), 15, false)
	if err != nil {
		t.Fatalf("set collection: %v", err)
	}
	if enabled {
		t.Fatalf("回读的开关应为 false，得到 %v", enabled)
	}

	// 服务不存在：影响 0 行 → ErrNotFound，而不是静默成功。
	mock.ExpectExec(regexp.QuoteMeta("UPDATE assets_application_service")).
		WithArgs(sqlmock.AnyArg(), true, int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if _, err = service.SetServiceLogCollection(context.Background(), 999, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("服务不存在应报 ErrNotFound，得到 %v", err)
	}

	// 参数非法不读库。
	if _, err = service.SetServiceLogCollection(context.Background(), 0, true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("缺少服务 id 应报 ErrInvalid，得到 %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

// 守卫：总开关的 UPDATE 只许写这一列 + update_time。多写一列就可能顺手改到别的服务属性
// （比如把 name/application 一起覆盖成空），而"只改开关"的接口不该有这种能力。
func TestUpdateServiceLogCollectionOnlyTouchesItsColumn(t *testing.T) {
	content, err := readRepositoryFile("db/queries/mysql/assets.sql")
	if err != nil {
		t.Fatalf("read queries: %v", err)
	}
	statement := namedStatement(content, "UpdateApplicationServiceLogCollection")
	if statement == "" {
		t.Fatal("找不到 UpdateApplicationServiceLogCollection 语句")
	}
	if !strings.Contains(statement, "log_collection_enabled=sqlc.arg(log_collection_enabled)") {
		t.Fatalf("语句没有写 log_collection_enabled：\n%s", statement)
	}
	for _, column := range []string{"name=", "code=", "application_id=", "business_system_id=", "deployment_template_id="} {
		if strings.Contains(statement, column) {
			t.Fatalf("总开关接口不应触碰 %s：\n%s", column, statement)
		}
	}
}
