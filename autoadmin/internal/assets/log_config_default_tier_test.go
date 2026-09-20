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

// log-config 响应必须带上**服务级默认保留档位**：界面上的"继承服务默认"要写成
// "继承服务默认（标准 30 天）"，只回一个"继承"没人知道这条日志实际保留多久（现场反馈）。
//
// 这条用例同时钉住"默认档位与采集总开关是同一条单行读取"（省一次往返），
// 以及"档位为空 = 由平台默认档决定"这个语义（null 要原样传出去，不能变成 0）。
func TestApplicationServiceLogConfigCarriesServiceDefaultTier(t *testing.T) {
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

	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(overrideFixtureRows(sql.NullString{}, nil, sql.NullInt64{}))
	// 服务级默认值：采集开 + 默认档位 4。
	mock.ExpectQuery(regexp.QuoteMeta("SELECT log_collection_enabled, log_retention_tier_id FROM assets_application_service")).
		WithArgs(int64(15)).
		WillReturnRows(sqlmock.NewRows([]string{"log_collection_enabled", "log_retention_tier_id"}).AddRow(true, int64(4)))
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_service")).
		WithArgs(int64(15)).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("nginx"))

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: "15"}}
	context.Request = httptest.NewRequest("GET", "/assets/application-services/15/log-config/", bytes.NewReader(nil))

	handler.GetApplicationServiceLogConfig(context)

	var payload struct {
		Data ServiceLogConfig `json:"data"`
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析响应体: %v（body=%s）", err, recorder.Body.String())
	}
	if payload.Data.LogRetentionTier == nil || *payload.Data.LogRetentionTier != 4 {
		t.Fatalf("响应应带服务默认档位 4，得到 %+v（body=%s）", payload.Data.LogRetentionTier, recorder.Body.String())
	}
	if !payload.Data.LogCollectionEnabled {
		t.Fatalf("总开关应照常带上：%s", recorder.Body.String())
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

// 服务没有配默认档位时：**原样回 null**（= 由平台默认档决定），不能变成 0——
// 0 会让界面显示成"档位 #0"或者把"没有默认"说成"有个档位"。
func TestApplicationServiceLogConfigKeepsNullDefaultTier(t *testing.T) {
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

	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(overrideFixtureRows(sql.NullString{}, nil, sql.NullInt64{}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT log_collection_enabled, log_retention_tier_id FROM assets_application_service")).
		WithArgs(int64(15)).
		WillReturnRows(sqlmock.NewRows([]string{"log_collection_enabled", "log_retention_tier_id"}).AddRow(true, nil))
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_service")).
		WithArgs(int64(15)).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("nginx"))

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: "15"}}
	context.Request = httptest.NewRequest("GET", "/assets/application-services/15/log-config/", bytes.NewReader(nil))

	handler.GetApplicationServiceLogConfig(context)

	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析响应体: %v（body=%s）", err, recorder.Body.String())
	}
	if value, ok := payload.Data["log_retention_tier"]; !ok || value != nil {
		t.Fatalf("没有服务默认档位时应回 null，得到 %#v", value)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}
