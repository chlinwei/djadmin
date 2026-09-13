package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// 覆盖 webhook 入库前置步骤：/api/v1/rules → fingerprint/alertname 双索引（含 PromQL query）。
func TestPrometheusAlertRuleIndexes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rules" {
			t.Errorf("unexpected upstream path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"groups":[{"name":"disk","rules":[
			{"name":"HighDiskUsage","query":"node_filesystem_avail_bytes/node_filesystem_size_bytes*100 < 10","duration":600,
			 "labels":{"severity":"warning"},"annotations":{"summary":"磁盘空间不足"},
			 "alerts":[{"labels":{"alertname":"HighDiskUsage","instance":"10.25.66.150:9100","host_id":"221"}}]},
			{"name":"AgentDown","query":"up == 0","duration":300,"labels":{},"annotations":{}}
		]}]}}`))
	}))
	defer upstream.Close()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = database.Close() }()
	mock.ExpectQuery("SELECT value FROM sys_config").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(upstream.URL))

	handler := &Handler{db: database, client: upstream.Client()}
	indexes := handler.prometheusAlertRuleIndexes(context.Background())

	rule, ok := indexes.byAlertname["HighDiskUsage"]
	if !ok {
		t.Fatalf("alertname index missing HighDiskUsage: %+v", indexes.byAlertname)
	}
	if rule.Query == "" || rule.GroupName != "disk" {
		t.Fatalf("rule details incomplete: %+v", rule)
	}
	// 指纹来自规则的 active alerts labels，与 alertFingerprint 同算法
	labels := map[string]any{"alertname": "HighDiskUsage", "instance": "10.25.66.150:9100", "host_id": "221"}
	if group := indexes.byFingerprint[alertFingerprint(labels)]; group != "disk" {
		t.Fatalf("fingerprint index group = %q, want disk", group)
	}
	if _, exists := indexes.byAlertname["AgentDown"]; !exists {
		t.Fatalf("alertname index missing AgentDown")
	}
}

// rules 接口失败时返回空索引，不阻塞告警入库（与 Django 版语义一致）。
func TestPrometheusAlertRuleIndexesUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = database.Close() }()
	mock.ExpectQuery("SELECT value FROM sys_config").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(upstream.URL))

	handler := &Handler{db: database, client: upstream.Client()}
	indexes := handler.prometheusAlertRuleIndexes(context.Background())
	if len(indexes.byAlertname) != 0 || len(indexes.byFingerprint) != 0 {
		t.Fatalf("expected empty indexes, got %+v", indexes)
	}
}

// 规则快照可序列化为前端期望的 {query, labels, annotations, ...} 结构。
func TestPrometheusAlertRuleSnapshotJSON(t *testing.T) {
	rule := prometheusAlertRule{GroupName: "disk", Name: "HighDiskUsage", Query: "up == 0", Duration: float64(600), Labels: map[string]any{"severity": "warning"}, Annotations: map[string]any{"summary": "磁盘空间不足"}}
	encoded, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["query"] != rule.Query || decoded["group_name"] != "disk" {
		t.Fatalf("unexpected snapshot json: %s", encoded)
	}
}
