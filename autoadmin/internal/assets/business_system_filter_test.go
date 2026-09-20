package assets

import (
	"bytes"
	"database/sql/driver"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// 业务系统列表的 `project` 过滤必须真的走到 SQL（2026-09-20 现场：服务树的「项目」节点下列出的是
// **全部**业务系统）。
//
// 这类 bug 的成因不是"算错"，而是**参数传了没人理**：前端一直传 `{project: id}`，而列表查询里
// 没有这个条件 —— 不报错、不告警，只是结果多了一圈（还连带把项目下聚合的服务数/实例数算错）。
// 所以这条用例断言的是"参数进了 SQL"（而不是"过滤逻辑对不对"，那由 SQL 本身保证）。
func TestListBusinessSystemsPassesProjectFilterToSQL(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))
	handler := NewHandler(service, nil, "")

	// 传参形态：搜索词在 SQL 里出现在 5 个占位符上、项目过滤 2 个（`= ? OR ? IS NULL`），
	// 生成的 Params 把它们各合成一个字段、由生成代码重复传同一个值。这里逐个写出，
	// 顺带钉住"两处 project 占位符都拿到了值"——漏一处就是"筛了等于没筛"。
	pattern := sqlmock.AnyArg()
	countArgs := []driver.Value{pattern, pattern, pattern, pattern, pattern, int64(301), int64(301)}
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_business_system s LEFT JOIN assets_project p")).
		WithArgs(countArgs...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_business_system s LEFT JOIN assets_project p")).
		WithArgs(append(append([]driver.Value{}, countArgs...), sqlmock.AnyArg(), sqlmock.AnyArg())...).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "create_time", "update_time", "remark", "name", "code", "owner", "enabled",
			"project_id", "project_name", "project_code",
		}).AddRow(int64(7), testRuleUpdatedAt, testRuleUpdatedAt, nil, "订单系统", "order-system", "ops", true,
			int64(301), "订单项目", "order-project"))

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/assets/business-systems/?project=301", bytes.NewReader(nil))

	handler.ListBusinessSystems(context)

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("project 参数没有走到 SQL（前端传了、后端当没看见 → 下层列出全部业务系统）: %v", err)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("订单系统")) {
		t.Fatalf("响应应含该项目下的业务系统：%s", recorder.Body.String())
	}
}

// 不传 project = 不筛选（与其它列表接口"缺省不筛选"一致）：传 NULL 而不是某个 id。
func TestListBusinessSystemsWithoutProjectDoesNotFilter(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))
	handler := NewHandler(service, nil, "")

	pattern := sqlmock.AnyArg()
	countArgs := []driver.Value{pattern, pattern, pattern, pattern, pattern, nil, nil}
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_business_system s LEFT JOIN assets_project p")).
		WithArgs(countArgs...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_business_system s LEFT JOIN assets_project p")).
		WithArgs(append(append([]driver.Value{}, countArgs...), sqlmock.AnyArg(), sqlmock.AnyArg())...).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "create_time", "update_time", "remark", "name", "code", "owner", "enabled",
			"project_id", "project_name", "project_code",
		}))

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/assets/business-systems/", bytes.NewReader(nil))

	handler.ListBusinessSystems(context)

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

// `project` 传了但不是整数 → 400，不静默当全量（静默会让人以为"筛选生效了，只是没数据"）。
func TestListBusinessSystemsRejectsInvalidProject(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))
	handler := NewHandler(service, nil, "")

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/assets/business-systems/?project=abc", bytes.NewReader(nil))

	handler.ListBusinessSystems(context)

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("非法 project 不该读库: %v", err)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":400`)) {
		t.Fatalf("非法 project 应报 400：%s", recorder.Body.String())
	}
}
