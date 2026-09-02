package monitor

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

const openSearchClusterQuery = `SELECT id,hosts,username,password,verify_tls,ca_cert,index_prefix,request_timeout FROM monitor_opensearch_cluster WHERE id=?`

// 回归用例：OpenSearch 的 _simulate API 要求每个 doc 是 {"_source": {...}}，
// 之前 SimulateOpenSearchPipeline 直接透传前端的 docs 数组，触发
// "[_source] required property is missing"。
func TestSimulateOpenSearchPipelineWrapsDocsWithSource(t *testing.T) {
	var capturedBody map[string]any
	openSearchServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode upstream request body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"docs":[{"doc":{"_source":{"message":"hello"}}}]}`))
	}))
	defer openSearchServer.Close()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	mock.ExpectQuery(regexp.QuoteMeta(openSearchClusterQuery)).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "hosts", "username", "password", "verify_tls", "ca_cert", "index_prefix", "request_timeout"},
	).AddRow(int64(1), openSearchServer.URL, "", "", false, "", "logs", 5))

	secrets, err := assets.NewSecretEncryptor("", "test-secret")
	if err != nil {
		t.Fatalf("create secret encryptor: %v", err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database, secrets: secrets}
	engine.POST("/opensearch-clusters/:id/pipeline-simulate/", handler.SimulateOpenSearchPipeline)

	requestBody := `{"pipeline":{"processors":[]},"docs":[{"message":"hello"}]}`
	request := httptest.NewRequest(http.MethodPost, "/opensearch-clusters/1/pipeline-simulate/", strings.NewReader(requestBody))
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
