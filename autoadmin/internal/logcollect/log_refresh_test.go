package logcollect

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"autoadmin/internal/assets"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

const (
	logRefreshDimsQuery  = `SELECT p.code AS project_code, e.code AS environment_code, bs.code AS business_system_code, s.code AS service_code`
	logRefreshTiersQuery = `SELECT DISTINCT s.id AS service_id, s.code AS service_code,`
)

// 刷新范围必须严格是"该服务采集中的 data stream"：档位来自 ListServiceActiveStreamTiers
// 且只取该服务编码的档位；cluster 的 index_prefix 决定流名前缀。
// 断言 ES 收到的是一次针对精确流名列表的 _refresh（不是全量 url）。
func TestElasticsearchLogRefreshServiceCollectingStreams(t *testing.T) {
	var requestedPaths []string
	elasticsearchServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		requestedPaths = append(requestedPaths, request.URL.Path+"?"+request.URL.RawQuery)
		_, _ = writer.Write([]byte(`{"_shards":{"total":2,"successful":2,"failed":0}}`))
	}))
	defer elasticsearchServer.Close()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	mock.ExpectQuery(regexp.QuoteMeta(elasticsearchClusterQuery)).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "hosts", "username", "password", "verify_tls", "ca_cert", "index_prefix", "request_timeout", "enabled"},
	).AddRow(int64(1), elasticsearchServer.URL, "", "", false, "", "autoadmin", 5, true))
	mock.ExpectQuery(regexp.QuoteMeta(logRefreshDimsQuery)).WillReturnRows(
		sqlmock.NewRows([]string{"project_code", "environment_code", "business_system_code", "service_code"}).
			AddRow("kul", "test", "tib", "openresty"),
	)
	// 该服务两个生效档位（多条日志同档位会去重，这里已是去重后的集合）；另一服务的档位不得混入。
	mock.ExpectQuery(regexp.QuoteMeta(logRefreshTiersQuery)).WillReturnRows(
		sqlmock.NewRows([]string{"service_id", "service_code", "tier_code"}).
			AddRow(int64(42), "openresty", "wuhan-test").
			AddRow(int64(42), "openresty", "hot").
			AddRow(int64(7), "other-svc", "std"),
	)

	secrets, err := assets.NewSecretEncryptor("", "test-secret")
	if err != nil {
		t.Fatalf("create secret encryptor: %v", err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database, secrets: secrets}
	engine.POST("/elasticsearch-clusters/:id/log-refresh/", handler.ElasticsearchLogRefresh)

	request := httptest.NewRequest(http.MethodPost, "/elasticsearch-clusters/1/log-refresh/",
		strings.NewReader(`{"application_service_id": 42}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if len(requestedPaths) != 1 {
		t.Fatalf("ES 请求次数 = %d，want 1；paths=%v", len(requestedPaths), requestedPaths)
	}
	// 档位按字母序（hot < wuhan-test）拼成逗号分隔的流名列表。
	const wantPath = `/autoadmin-kul-tib-test-openresty-hot,autoadmin-kul-tib-test-openresty-wuhan-test/_refresh`
	if got := strings.SplitN(requestedPaths[0], "?", 2)[0]; got != wantPath {
		t.Fatalf("refresh path = %q, want %q", got, wantPath)
	}
	if !strings.Contains(requestedPaths[0], "ignore_unavailable=true") {
		t.Fatalf("refresh query 缺少 ignore_unavailable: %q", requestedPaths[0])
	}
	if strings.Contains(recorder.Body.String(), "other-svc") {
		t.Fatalf("刷新范围混入了其它服务：%s", recorder.Body.String())
	}
}

// 没有生效档位（未挂日志定义/无采集中的流）时不发任何 ES 请求，明确返回 count=0。
func TestElasticsearchLogRefreshNoCollectingStreams(t *testing.T) {
	elasticsearchHit := false
	elasticsearchServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		elasticsearchHit = true
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{}`))
	}))
	defer elasticsearchServer.Close()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	mock.ExpectQuery(regexp.QuoteMeta(elasticsearchClusterQuery)).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "hosts", "username", "password", "verify_tls", "ca_cert", "index_prefix", "request_timeout", "enabled"},
	).AddRow(int64(1), elasticsearchServer.URL, "", "", false, "", "autoadmin", 5, true))
	mock.ExpectQuery(regexp.QuoteMeta(logRefreshDimsQuery)).WillReturnRows(
		sqlmock.NewRows([]string{"project_code", "environment_code", "business_system_code", "service_code"}).
			AddRow("kul", "test", "tib", "openresty"),
	)
	mock.ExpectQuery(regexp.QuoteMeta(logRefreshTiersQuery)).WillReturnRows(
		sqlmock.NewRows([]string{"service_id", "service_code", "tier_code"}).AddRow(int64(7), "other-svc", "std"),
	)

	secrets, err := assets.NewSecretEncryptor("", "test-secret")
	if err != nil {
		t.Fatalf("create secret encryptor: %v", err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database, secrets: secrets}
	engine.POST("/elasticsearch-clusters/:id/log-refresh/", handler.ElasticsearchLogRefresh)

	request := httptest.NewRequest(http.MethodPost, "/elasticsearch-clusters/1/log-refresh/",
		strings.NewReader(`{"application_service_id": 42}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if elasticsearchHit {
		t.Fatalf("无采集中的流时不应请求 ES")
	}
	if !strings.Contains(recorder.Body.String(), `"count":0`) {
		t.Fatalf("body 应包含 count=0：%s", recorder.Body.String())
	}
}

// 服务不存在（维度查询无行）→ 404，且不请求 ES。
func TestElasticsearchLogRefreshServiceNotFound(t *testing.T) {
	elasticsearchHit := false
	elasticsearchServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		elasticsearchHit = true
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{}`))
	}))
	defer elasticsearchServer.Close()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	mock.ExpectQuery(regexp.QuoteMeta(elasticsearchClusterQuery)).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "hosts", "username", "password", "verify_tls", "ca_cert", "index_prefix", "request_timeout", "enabled"},
	).AddRow(int64(1), elasticsearchServer.URL, "", "", false, "", "autoadmin", 5, true))
	mock.ExpectQuery(regexp.QuoteMeta(logRefreshDimsQuery)).WillReturnRows(
		sqlmock.NewRows([]string{"project_code", "environment_code", "business_system_code", "service_code"}),
	)

	secrets, err := assets.NewSecretEncryptor("", "test-secret")
	if err != nil {
		t.Fatalf("create secret encryptor: %v", err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database, secrets: secrets}
	engine.POST("/elasticsearch-clusters/:id/log-refresh/", handler.ElasticsearchLogRefresh)

	request := httptest.NewRequest(http.MethodPost, "/elasticsearch-clusters/1/log-refresh/",
		strings.NewReader(`{"application_service_id": 999}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"code":404`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if elasticsearchHit {
		t.Fatalf("服务不存在时不应请求 ES")
	}
}
