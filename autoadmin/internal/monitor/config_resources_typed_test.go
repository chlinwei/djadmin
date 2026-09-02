package monitor

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"testing"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// 回归用例：pipeline_body 现在是 sqlc 生成的 json.RawMessage，序列化到 JSON 响应里必须是
// 真正的对象（{"processors":[...]}），不能像旧的 map[string]any 手写扫描那样被 Gin 二次转义
// 成带反斜杠的字符串。
func TestProcessingRuleResponseSerializesPipelineBodyAsRealJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/", nil)
	handler := &Handler{}
	row := db.MonitorLogProcessingRule{
		ID:               1,
		Name:             "rule-a",
		MultilineEnabled: true,
		PipelineBody:     json.RawMessage(`{"processors":[{"set":{"field":"a","value":"b"}}]}`),
	}

	encoded, err := json.Marshal(handler.processingRuleResponse(context, row))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	body, ok := decoded["pipeline_body"].(map[string]any)
	if !ok {
		t.Fatalf("pipeline_body type = %T, want a JSON object, not a re-escaped string", decoded["pipeline_body"])
	}
	if _, ok := body["processors"].([]any); !ok {
		t.Fatalf("pipeline_body.processors missing or wrong type: %#v", body["processors"])
	}
	if decoded["multiline_enabled"] != true {
		t.Fatalf("multiline_enabled = %#v, want JSON boolean true", decoded["multiline_enabled"])
	}
}

// 回归用例：monitor_log_retention_tier 的 enabled/is_default 必须序列化成 JSON 布尔值，
// 不能是数字 0/1（这正是 monitor_target 那次 bug 的同类问题，这里用 sqlc 生成的 bool 字段验证已修复）。
func TestRetentionTierResponseSerializesBooleansAsJSONBooleans(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM assets_application_service WHERE log_retention_tier_id=?`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/", nil)
	handler := &Handler{db: database}
	row := db.MonitorLogRetentionTier{ID: 1, Code: "hot", Enabled: true, IsDefault: false}

	encoded, err := json.Marshal(handler.retentionTierResponse(context, row))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if decoded["enabled"] != true {
		t.Fatalf("enabled = %#v, want JSON boolean true", decoded["enabled"])
	}
	if decoded["is_default"] != false {
		t.Fatalf("is_default = %#v, want JSON boolean false", decoded["is_default"])
	}
}
