package assets

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// applicationServiceDetailColumns / RowValues：GetApplicationServiceDetail 的列序（与 SQL 的 SELECT 一致）。
// 列序错了会被 Scan 报出来，不会静默错值。
func applicationServiceDetailColumns() []string {
	return []string{
		"id", "create_time", "update_time", "remark", "name", "code", "topology_type", "access_address", "enabled",
		"application_id", "application_name", "business_system_id", "business_system_name",
		"environment_id", "environment_name", "application_version_id", "application_version_name",
		"deployment_template_id", "deployment_template_name", "cluster_profile_id", "cluster_profile_name",
		"macro_values", "log_collection_enabled", "log_retention_tier_id", "deployment_count",
	}
}

func applicationServiceDetailRowValues(id, templateID int64) []driver.Value {
	return []driver.Value{
		id, testRuleUpdatedAt, testRuleUpdatedAt, nil, "订单 API", "order-api", "standalone", "", true,
		int64(5), "Order API", int64(7), "订单系统",
		int64(31), "生产环境", int64(51), "1.0",
		templateID, "Order Template", nil, "",
		[]byte("{}"), true, nil, int64(2),
	}
}

// 服务详情必须带上**监听端口**（2026-09-20 现场：服务树的「监听端口」一节永远显示"未配置端口"）。
//
// 端口只在部署模板上定义（`assets_application_port`），服务侧只继承不单独维护，所以服务详情要按
// 服务的 `deployment_template` 反查一次。前端一直在渲染 `entity.ports`（名称 · 协议 端口），
// 只是后端从来没把这个字段带出来——单测里 mock 了 ports 所以看不出来，这条用例把它钉在接口上。
func TestApplicationServiceDetailCarriesTemplatePorts(t *testing.T) {
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

	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_service s")).
		WithArgs(int64(21)).
		WillReturnRows(sqlmock.NewRows(applicationServiceDetailColumns()).
			AddRow(applicationServiceDetailRowValues(21, 62)...))
	// 端口按服务所属模板（62）来取，且顺序由 SQL 给（protocol, port）。
	// 服务详情自身的第二次查询：成员实例（顺序要在端口之前，因为 GetApplicationService 内部先读它）。
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_service_deployment WHERE service_id=")).
		WithArgs(int64(21)).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}).AddRow(int64(31)))
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_port WHERE deployment_template_id=")).
		WithArgs(int64(62)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "create_time", "update_time", "remark", "name", "protocol", "bind_address", "port",
			"required", "external_access", "check_enabled",
		}).
			AddRow(int64(1), testRuleUpdatedAt, testRuleUpdatedAt, nil, "HTTP", "tcp", "0.0.0.0", int64(8080), true, true, true).
			AddRow(int64(2), testRuleUpdatedAt, testRuleUpdatedAt, nil, "管理端口", "tcp", "127.0.0.1", int64(9000), false, false, true))

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: "21"}}
	context.Request = httptest.NewRequest("GET", "/assets/application-services/21/", bytes.NewReader(nil))

	handler.GetApplicationService(context)

	var payload struct {
		Data ApplicationService `json:"data"`
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析响应体: %v（body=%s）", err, recorder.Body.String())
	}
	ports := payload.Data.Ports
	if len(ports) != 2 {
		t.Fatalf("服务详情应带 2 个端口，得到 %d（body=%s）", len(ports), recorder.Body.String())
	}
	// 前端渲染的就是这三样：名称、协议、端口。
	if ports[0].Name != "HTTP" || ports[0].Protocol != "tcp" || ports[0].Port != 8080 {
		t.Fatalf("端口字段不对：%+v", ports[0])
	}
	if ports[1].Name != "管理端口" || ports[1].Port != 9000 {
		t.Fatalf("第二个端口不对：%+v", ports[1])
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

// 读端口失败**不能静默**：静默的后果就是界面显示"未配置端口"（那正是这次要修的表现）。
func TestApplicationServiceDetailFailsWhenPortsUnreadable(t *testing.T) {
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

	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_service s")).
		WithArgs(int64(21)).
		WillReturnRows(sqlmock.NewRows(applicationServiceDetailColumns()).
			AddRow(applicationServiceDetailRowValues(21, 62)...))
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_service_deployment WHERE service_id=")).
		WithArgs(int64(21)).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}).AddRow(int64(31)))
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_port WHERE deployment_template_id=")).
		WithArgs(int64(62)).
		WillReturnError(sql.ErrConnDone)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: "21"}}
	context.Request = httptest.NewRequest("GET", "/assets/application-services/21/", bytes.NewReader(nil))

	handler.GetApplicationService(context)

	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("解析响应体: %v（body=%s）", err, recorder.Body.String())
	}
	if envelope.Code == 200 {
		t.Fatalf("读端口失败应报错，而不是回一份「没有端口」的详情：%s", recorder.Body.String())
	}
}
