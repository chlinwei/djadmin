package assets

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// log-config 响应必须带上**服务级默认保留档位**：界面上的"继承服务默认"要写成
// "继承服务默认（标准 30 天）"，只回一个"继承"没人知道这条日志实际保留多久（现场反馈）。
//
// 这条用例同时钉住"默认档位与采集总开关是同一条单行读取"（省一次往返），
// 以及旧记录 null 要如实暴露、不能变成 0 或猜测平台默认档。
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
	expectNoLogFormatInstances(mock)
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

// 旧服务没有配默认档位时：原样回 null，供界面提示补选，不能变成 0——
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
	expectNoLogFormatInstances(mock)
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

func TestSaveApplicationServiceRequiresRetentionTier(t *testing.T) {
	for _, id := range []int64{0, 15} {
		for _, tier := range []*int64{nil, new(int64(0)), new(int64(-1))} {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			repository := NewRepository(database)
			_, err = repository.SaveApplicationService(context.Background(), id, ApplicationServiceInput{LogRetentionTier: tier}, nil)
			if err == nil || !strings.Contains(err.Error(), "不能留空") {
				t.Fatalf("id=%d tier=%v: expected required tier error, got %v", id, tier, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
			database.Close()
		}
	}
}

func TestEffectiveRetentionQueriesDoNotGuessPlatformDefault(t *testing.T) {
	for _, test := range []struct{ path, name string }{
		{"db/queries/mysql/assets.sql", "ListServiceTemplateLogs"},
		{"db/queries/mysql/monitor.sql", "ListHostLogRenderEntries"},
		{"db/queries/mysql/monitor.sql", "ListServiceActiveStreamTiers"},
	} {
		content, err := readRepositoryFile(test.path)
		if err != nil {
			t.Fatal(err)
		}
		query := namedStatement(content, test.name)
		for _, required := range []string{
			"tier.id = COALESCE(ls.retention_tier_id, s.log_retention_tier_id)",
			"tier.enabled = TRUE",
		} {
			if !strings.Contains(query, required) {
				t.Errorf("%s missing effective tier constraint %q", test.name, required)
			}
		}
		if strings.Contains(query, "is_default") || strings.Contains(query, "'std'") {
			t.Errorf("%s must not guess a platform default tier", test.name)
		}
		if strings.Contains(query, "s.log_retention_tier_id IS NOT NULL") {
			t.Errorf("%s must allow an explicit log override without a service default", test.name)
		}
	}
}

func TestSaveApplicationServiceRejectsInvalidRetentionTier(t *testing.T) {
	for _, state := range []string{"missing", "disabled", "empty-code", "query-error"} {
		t.Run(state, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}

			defer database.Close()
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT id FROM assets_business_system WHERE").
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM assets_service_deletion").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			query := mock.ExpectQuery("FROM monitor_log_retention_tier").WithArgs(int64(3))
			switch state {
			case "missing":
				query.WillReturnError(sql.ErrNoRows)
			case "query-error":
				query.WillReturnError(fmt.Errorf("tier lookup failed"))
			case "disabled":
				query.WillReturnRows(serviceRetentionTierRows("std", false))
			case "empty-code":
				query.WillReturnRows(serviceRetentionTierRows(" ", true))
			}
			mock.ExpectRollback()
			_, err = NewRepository(database).SaveApplicationService(context.Background(), 0,
				ApplicationServiceInput{BusinessSystem: 1, LogRetentionTier: new(int64(3))}, nil)
			if err == nil {
				t.Fatal("invalid tier must prevent writing the service")
			}
			if state == "query-error" && !strings.Contains(err.Error(), "tier lookup failed") {
				t.Fatalf("database errors must remain visible: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
