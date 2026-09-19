package assets

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// 按行保存日志覆盖值时，**请求体里的每一列都必须真的走到 SQL**。
//
// 2026-09-19 现场：handler 的绑定结构里漏了采集过滤的两列，前端选完过滤后刷新"没保存"——
// 请求带着它，绑定丢掉了，upsert 就把那一列写成 NULL；更糟的是此后改任何一列都会顺手清掉它。
// 这条用例按"字段逐项对齐"来钉：json tag 改名、少一个字段都会红。
func TestSaveApplicationServiceLogSettingPassesEveryOverrideColumn(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service, err := NewService(NewRepository(database), "", "test-django-secret")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	handler := NewHandler(service, nil, "")

	// 1) 回读该行（校验"这条日志属于本服务模板"）
	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(overrideFixtureRows(sql.NullString{}, nil, sql.NullInt64{}))
	// 2) 按行 upsert：四个覆盖列都要带上请求体里的值
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO assets_application_service_log_setting")).
		WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), int64(24), sqlmock.AnyArg(), int64(15),
			int64(7), int64(0),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 3) 回读整行返回给前端
	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(overrideFixtureRows(sql.NullString{}, nil, sql.NullInt64{}))

	body, _ := json.Marshal(map[string]any{
		"log_definition_id": 24,
		// include 选规则 7、exclude 显式关闭（0）——0 与 null 必须区分开
		"collection_filter_rule":         7,
		"collection_exclude_filter_rule": 0,
	})
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: "15"}}
	context.Request = httptest.NewRequest("POST", "/assets/application-services/15/log-config/settings/", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	handler.SaveApplicationServiceLogSetting(context)

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("请求体里的覆盖列没有全部走到 SQL（漏列会静默清空那一列）: %v", err)
	}
}
