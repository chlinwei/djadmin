package logcollect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"autoadmin/internal/assets"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// 片段到 WHERE 为止：占位符形态两侧不同（`?` / `$1`），断言不依赖它。
const elasticsearchClusterQuery = `SELECT id,hosts,username,password,verify_tls,ca_cert,index_prefix,request_timeout,enabled FROM monitor_elasticsearch_cluster`

// 发布前静态校验：pipeline 必须会写入 error_fingerprint（fingerprint/set/copy/rename 任一命中）。
func TestPipelineWritesErrorFingerprint(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
		want bool
	}{
		{"fingerprint.target_field", map[string]any{"processors": []any{map[string]any{"fingerprint": map[string]any{"fields": []any{"a"}, "target_field": "error_fingerprint"}}}}, true},
		{"set.field", map[string]any{"processors": []any{map[string]any{"set": map[string]any{"field": "error_fingerprint", "value": "x"}}}}, true},
		{"rename.target_field", map[string]any{"processors": []any{map[string]any{"rename": map[string]any{"field": "a", "target_field": "error_fingerprint"}}}}, true},
		{"missing", map[string]any{"processors": []any{map[string]any{"grok": map[string]any{"field": "message"}}}}, false},
		{"fingerprint other target", map[string]any{"processors": []any{map[string]any{"fingerprint": map[string]any{"fields": []any{"a"}}}}}, false},
	}
	for _, testCase := range cases {
		if got := pipelineWritesErrorFingerprint(testCase.body); got != testCase.want {
			t.Errorf("%s: pipelineWritesErrorFingerprint = %v, want %v", testCase.name, got, testCase.want)
		}
	}
}

// 字段校验按传入的 mapping 字段集，而不是硬编码。
func TestNonStandardDocumentFieldsUsesMapping(t *testing.T) {
	result := map[string]any{"docs": []any{map[string]any{"doc": map[string]any{"_source": map[string]any{
		"message": "x", "log": "y", "app_fields": map[string]any{"pid": "1"},
	}}}}}
	allowed := map[string]bool{"message": true, "app_fields": true}
	got := nonStandardDocumentFields(result, allowed)
	if len(got) != 1 || got[0] != "log" {
		t.Fatalf("nonStandardDocumentFields = %v, want [log]", got)
	}
	if missing := missingRequiredDocumentFields(result); len(missing) != 1 || missing[0] != "error_fingerprint" {
		t.Fatalf("missingRequiredDocumentFields = %v", missing)
	}
}

// 回归用例：Elasticsearch 的 _simulate API 要求每个 doc 是 {"_source": {...}}，
// 之前 SimulateElasticsearchPipeline 直接透传前端的 docs 数组，触发
// "[_source] required property is missing"。
func TestSimulateElasticsearchPipelineWrapsDocsWithSource(t *testing.T) {
	var capturedBody map[string]any
	elasticsearchServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		// 校验会先 GET 索引模板 mapping；mock 返回 404 走内置标准字段回退。
		if request.Method == http.MethodGet {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":"not found"}`))
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode upstream request body: %v", err)
		}
		_, _ = writer.Write([]byte(`{"docs":[{"doc":{"_source":{"message":"hello"}}}]}`))
	}))
	defer elasticsearchServer.Close()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	mock.ExpectQuery(regexp.QuoteMeta(elasticsearchClusterQuery)).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "hosts", "username", "password", "verify_tls", "ca_cert", "index_prefix", "request_timeout", "enabled"},
	).AddRow(int64(1), elasticsearchServer.URL, "", "", false, "", "logs", 5, true))

	secrets, err := assets.NewSecretEncryptor("", "test-secret")
	if err != nil {
		t.Fatalf("create secret encryptor: %v", err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database, secrets: secrets}
	engine.POST("/elasticsearch-clusters/:id/pipeline-simulate/", handler.SimulateElasticsearchPipeline)

	requestBody := `{"pipeline":{"processors":[]},"docs":[{"message":"hello"}]}`
	request := httptest.NewRequest(http.MethodPost, "/elasticsearch-clusters/1/pipeline-simulate/", strings.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	docs, ok := capturedBody["docs"].([]any)
	if !ok || len(docs) != 1 {
		t.Fatalf("upstream docs = %#v, want a single-element array", capturedBody["docs"])
	}
	doc, ok := docs[0].(map[string]any)
	if !ok {
		t.Fatalf("upstream doc[0] type = %T, want object", docs[0])
	}
	source, ok := doc["_source"].(map[string]any)
	if !ok {
		t.Fatalf(`upstream doc[0] = %#v, want a "_source" wrapper`, doc)
	}
	if source["message"] != "hello" {
		t.Fatalf("_source.message = %#v, want %q", source["message"], "hello")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}
