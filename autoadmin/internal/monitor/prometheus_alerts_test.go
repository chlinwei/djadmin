package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// 回归用例：当前告警必须带上规则组与规则详情（PromQL）。
//
// `/api/v1/alerts` 本身不含规则表达式（PromQL 只在 `/api/v1/rules`），所以 handler 需要
// 按 alertname 关联规则索引补 `rule_group` / `rule_details`；否则前端"当前告警"的规则组列
// 与展开行的 PromQL/标签/summary 恒为空（历史告警读落库快照，所以没有这个问题）。
func TestPrometheusAlertsIncludeRuleDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/alerts":
			_, _ = writer.Write([]byte(`{"status":"success","data":{"alerts":[{"labels":{"alertname":"HighDiskUsage","severity":"warning","instance":"h:9100"},"annotations":{"summary":"磁盘空间不足"},"state":"firing","activeAt":"2026-09-17T00:00:00Z","value":"1"}]}}`))
		case "/api/v1/rules":
			_, _ = writer.Write([]byte(`{"status":"success","data":{"groups":[{"name":"disk","file":"rules.yml","rules":[{"name":"HighDiskUsage","query":"node_filesystem_avail_bytes < 10","duration":600,"labels":{"severity":"warning"},"annotations":{"summary":"磁盘空间不足"},"alerts":[]}]}]}}`))
		default:
			t.Errorf("unexpected upstream path %q", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = database.Close() }()
	// 两次上游调用（/alerts 与 /rules）各查一次 base URL。
	mock.ExpectQuery("SELECT value FROM sys_config").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(server.URL))
	mock.ExpectQuery("SELECT value FROM sys_config").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(server.URL))

	handler := &Handler{db: database, client: server.Client()}
	router := gin.New()
	router.GET("/monitor/targets/prometheus/alerts/", handler.PrometheusAlerts)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/monitor/targets/prometheus/alerts/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v body=%s", err, recorder.Body.String())
	}
	var payload struct {
		Results []struct {
			RuleGroup   string `json:"rule_group"`
			RuleDetails *struct {
				Query string `json:"query"`
			} `json:"rule_details"`
		} `json:"results"`
	}
	if err := json.Unmarshal(envelope.Data, &payload); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if len(payload.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(payload.Results))
	}
	if payload.Results[0].RuleGroup != "disk" {
		t.Fatalf("rule_group = %q, want disk", payload.Results[0].RuleGroup)
	}
	if payload.Results[0].RuleDetails == nil || payload.Results[0].RuleDetails.Query == "" {
		t.Fatalf("rule_details.query 为空：%+v（当前告警必须补齐 PromQL）", payload.Results[0].RuleDetails)
	}
	assertConfigQueryConsumed(t, mock)
}
