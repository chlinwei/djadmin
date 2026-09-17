package assets

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"autoadmin/internal/shared/pagination"

	"github.com/DATA-DOG/go-sqlmock"
)

// notNilArg 只接受非 nil 的驱动参数。
//
// 用途：查询用 `sqlc.arg(x) = 0` 表示"不过滤"（BUG_SQLC_NULLABLE_FILTER 的约定），
// 这要求调用点传**有效的 0**；列可空会让 sqlc 把参数生成为 sql.NullInt64，若调用点
// 按"0 就传 NULL"处理，`NULL = 0` 求值为 NULL，整条 WHERE 变 NULL，列表恒空。
// 所以"不过滤"分支的参数必须非 nil。
type notNilArg struct{}

func (notNilArg) Match(value driver.Value) bool { return value != nil }

// 回归用例：集群模型列表在"未选应用"时必须返回全部，而不是恒空。
//
// 2026-09-17 线上现象：`GET /assets/cluster-profiles/?page=1&page_size=10&search=` 返回空，
// a-table 无数据。根因是 ListProfiles 把 applicationID=0 包成了 `sql.NullInt64{Valid:false}`
// （NULL），而 SQL 用 `sqlc.arg(application_id) = 0` 判"不过滤"（P5 陷阱 22）。
// 该用例钉住"不过滤分支传的是有效 0 而非 NULL"——参数为 NULL 时 mock 不匹配、用例失败。
func TestListProfilesWithoutApplicationFilterReturnsRows(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	page := pagination.New(1, 10)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM assets_cluster_profile")).
		WithArgs(clusterProfileCountArgs()...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT p.id, p.create_time")).
		WithArgs(clusterProfileListArgs(page.Size, page.Offset)...).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "create_time", "update_time", "remark", "name", "code", "profile_type",
			"enabled", "application_id", "cluster_type", "application_name", "service_count",
		}).AddRow(int64(9), time.Now().UTC(), time.Now().UTC(), nil, "默认集群", "default",
			"single", true, nil, "standalone", "核心应用", int32(0)))

	repository := NewRepository(database)
	rows, count, err := repository.ListProfiles(context.Background(), 0, "", page)
	if err != nil {
		t.Fatalf("ListProfiles(no filter) failed: %v", err)
	}
	if count != 1 || len(rows) != 1 {
		t.Fatalf("count=%d rows=%d, want 1/1（不过滤分支不得恒空）", count, len(rows))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}
