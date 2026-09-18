package logcollect

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// Filebeat 只按 CPU 架构匹配（官方 tar.gz 单二进制，自带依赖）。
func TestNormalizeHostArch(t *testing.T) {
	cases := map[string]string{
		"x86_64": "amd64", "AMD64": "amd64", "aarch64": "arm64", "arm64": "arm64",
		"": "", "ppc64le": "",
	}
	for input, want := range cases {
		if got := normalizeHostArch(input); got != want {
			t.Fatalf("normalizeHostArch(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFirstElasticsearchURL(t *testing.T) {
	url, err := firstElasticsearchURL("https://10.0.0.1:9200,https://10.0.0.2:9200")
	if err != nil || url != "https://10.0.0.1:9200" {
		t.Fatalf("firstElasticsearchURL = %s, %v", url, err)
	}
	url, err = firstElasticsearchURL("10.0.0.3")
	if err != nil || url != "http://10.0.0.3:9200" {
		t.Fatalf("default scheme/port = %s, %v", url, err)
	}
	if _, err := firstElasticsearchURL(""); err == nil {
		t.Fatal("empty hosts should fail")
	}
}

func newLogTargetTestServer(t *testing.T, database *sql.DB) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database}
	engine.POST("/log-targets/:id/retry/", handler.RetryLogTarget)
	return engine
}

// 派发守卫回归：目标不存在必须 404；agent 离线必须 400，
// 不允许像旧 exporter 安装链路那样把失败静默成成功。
func TestRetryLogTargetGuards(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	// 片段到 WHERE 为止：占位符形态两侧不同（`?` / `$1`），断言不依赖它。
	selectQuery := regexp.QuoteMeta(`SELECT l.id, l.host_id, l.managed_enabled, l.install_status, COALESCE(h.instance_name, ''), COALESCE(h.ip, ''), COALESCE(s.os_type, ''), COALESCE(s.os_id_like, ''), COALESCE(s.os_version_id, '') FROM monitor_log_collection_target l JOIN assets_host h ON h.id = l.host_id LEFT JOIN assets_hostsystem s ON s.host_id = l.host_id`)

	gin.SetMode(gin.TestMode)

	// 404：目标不存在
	engine := newLogTargetTestServer(t, database)
	mock.ExpectQuery(selectQuery).WithArgs(int64(999)).WillReturnError(sql.ErrNoRows)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/log-targets/999/retry/", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"code":404`) {
		t.Fatalf("missing target: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	// 400：agent 离线（gateway 为 nil 视同离线）
	mock.ExpectQuery(selectQuery).WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "host_id", "managed_enabled", "install_status", "instance_name", "ip", "os_type", "os_id_like", "os_version_id"}).
			AddRow(3, 221, true, "success", "localhost", "10.25.66.150", "Ubuntu", "debian", "22.04"))
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/log-targets/3/retry/", nil))
	if !strings.Contains(recorder.Body.String(), "host agent is offline") {
		t.Fatalf("offline guard: body=%s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
