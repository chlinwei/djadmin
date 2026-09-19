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

// 发布前静态校验：pipeline 必须会写入三个必备字段（log_level / log_message / error_fingerprint），
// 覆盖 fingerprint/set/copy/rename 的 target_field/field，以及 dissect/grok 的命名捕获。
// 注意它是启发式（判不了条件分支），真正的强制在索引模板挂的 mapping-guard。
func TestMissingPipelineOutputs(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
		want []string
	}{
		{
			name: "全部齐（dissect 命名捕获 + fingerprint）",
			body: map[string]any{"processors": []any{
				map[string]any{"dissect": map[string]any{"field": "message", "pattern": "%{ts} %{log_level} %{log_message}"}},
				map[string]any{"fingerprint": map[string]any{"fields": []any{"a"}, "target_field": "error_fingerprint"}},
			}},
			want: nil,
		},
		{
			name: "grok 命名捕获也算",
			body: map[string]any{"processors": []any{
				map[string]any{"grok": map[string]any{"field": "message", "patterns": []any{"%{LOGLEVEL:log_level} %{GREEDYDATA:log_message}"}}},
				map[string]any{"set": map[string]any{"field": "error_fingerprint", "value": "x"}},
			}},
			want: nil,
		},
		{
			name: "只写了 error_fingerprint（历史规则的典型形态）",
			body: map[string]any{"processors": []any{
				map[string]any{"fingerprint": map[string]any{"fields": []any{"a"}, "target_field": "error_fingerprint"}},
			}},
			want: []string{"log_level", "log_message"},
		},
		{
			name: "rename 也算写入",
			body: map[string]any{"processors": []any{
				map[string]any{"rename": map[string]any{"field": "lvl", "target_field": "log_level"}},
				map[string]any{"set": map[string]any{"field": "log_message", "value": "x"}},
				map[string]any{"set": map[string]any{"field": "error_fingerprint", "value": "x"}},
			}},
			want: nil,
		},
		{"没有 processors", map[string]any{}, []string{"log_level", "log_message", "error_fingerprint"}},
	}
	for _, testCase := range cases {
		got := missingPipelineOutputs(testCase.body)
		if strings.Join(got, ",") != strings.Join(testCase.want, ",") {
			t.Errorf("%s: missingPipelineOutputs = %v, want %v", testCase.name, got, testCase.want)
		}
	}
}

// 字段校验按传入的 mapping 字段集，而不是硬编码。
// 违规样本用**规则自己造的字段**：Filebeat 自带字段（log/host/…）由 nonStandardDocumentFields
// 豁免（它们不在模板 mapping 里是有意为之，见 sample_event.go），拿 log 当样本测不到点子上。
func TestNonStandardDocumentFieldsUsesMapping(t *testing.T) {
	result := map[string]any{"docs": []any{map[string]any{"doc": map[string]any{"_source": map[string]any{
		"message": "x", "custom_field": "y", "app_fields": map[string]any{"pid": "1"},
	}}}}}
	allowed := map[string]bool{"message": true, "app_fields": true}
	got := nonStandardDocumentFields(result, allowed)
	if len(got) != 1 || got[0] != "custom_field" {
		t.Fatalf("nonStandardDocumentFields = %v, want [custom_field]", got)
	}
	// 三个必备字段一个都没有时全部列出（结果按字段名排序）。
	missing := missingRequiredDocumentFields(result)
	if strings.Join(missing, ",") != "error_fingerprint,log_level,log_message" {
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
