package monitor

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// 回归用例：index template 的 REST 名字必须是 `<prefix>-template`。
//
// 线上现象（2026-09-17）：链路体检报"索引模板 logs-template 不存在"，而集群里其实有
// Django 建的 `logs-template`。根因是 bootstrap 的 PUT 与体检的 GET 都误用了不带
// `-template` 后缀的 `<prefix>`，去查一个永远不存在的模板名（与 LOG_COLLECTION_ARCHITECTURE
// §4.4 的 `PUT _index_template/logs-template` 不符）。这里锁定 bootstrap 的上行路径。
func TestBootstrapIndexTemplateUsesTemplateSuffix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.Method+" "+request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			// 历史错名模板不存在 → cleanup 跳过。
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":"not found"}`))
			return
		}
		_, _ = writer.Write([]byte(`{}`))
	}))
	defer server.Close()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT code,retention_days,daily_size_gb,rollover_min_index_age")).
		WillReturnRows(sqlmock.NewRows([]string{"code", "retention_days", "daily_size_gb", "rollover_min_index_age"}))

	handler := &Handler{db: database}
	ginContext, _ := gin.CreateTestContext(nil)
	if err := handler.bootstrapElasticsearchStorage(ginContext, elasticsearchCluster{Hosts: server.URL, IndexPrefix: "logs", Timeout: 5}); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	joined := strings.Join(paths, "\n")
	if !strings.Contains(joined, "PUT /_index_template/logs-template") {
		t.Fatalf("bootstrap 未按 <prefix>-template 写模板:\n%s", joined)
	}
	if strings.Contains(joined, "PUT /_index_template/logs\n") {
		t.Fatalf("bootstrap 仍在用错名 /logs:\n%s", joined)
	}
}
