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

const serviceStreamDimsQuery = `FROM assets_application_service s`

// 数据流清理：服务端按 service_id 自行拼流名模式，mode 决定删除范围，走异步 _delete_by_query。
func TestCleanupLogDataStream(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		expectPath string
		expectBody string
	}{
		{
			name:       "all",
			body:       `{"service_id":17,"mode":"all"}`,
			expectPath: "/autoadmin-yilake-tib-poc-mgmt-*/_delete_by_query",
			expectBody: `"match_all"`,
		},
		{
			name:       "hours",
			body:       `{"service_id":17,"mode":"hours","amount":6}`,
			expectPath: "/autoadmin-yilake-tib-poc-mgmt-*/_delete_by_query",
			expectBody: `"lt":"now-6h"`,
		},
		{
			name:       "days",
			body:       `{"service_id":17,"mode":"days","amount":3}`,
			expectPath: "/autoadmin-yilake-tib-poc-mgmt-*/_delete_by_query",
			expectBody: `"lt":"now-3d"`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			code, result, capturedPath, capturedBody := runCleanup(t, testCase.body, http.StatusOK,
				`{"task":"node:1"}`)
			if code != 200 {
				t.Fatalf("envelope code = %d, data = %v", code, result)
			}
			if !strings.Contains(capturedPath, testCase.expectPath) {
				t.Fatalf("upstream path = %q, want contains %q", capturedPath, testCase.expectPath)
			}
			if !strings.Contains(capturedBody, testCase.expectBody) {
				t.Fatalf("upstream body = %q, want contains %q", capturedBody, testCase.expectBody)
			}
			if !strings.Contains(capturedPath, "wait_for_completion=false") {
				t.Fatalf("cleanup must be async: %q", capturedPath)
			}
			if result["stream_pattern"] != "autoadmin-yilake-tib-poc-mgmt-*" {
				t.Fatalf("stream_pattern = %v", result["stream_pattern"])
			}
			if result["matched"] != true {
				t.Fatalf("matched = %v", result["matched"])
			}
		})
	}
	t.Run("no matching datastream", func(t *testing.T) {
		code, result, _, _ := runCleanup(t, `{"service_id":17,"mode":"all"}`, http.StatusNotFound,
			`{"error":{"type":"index_not_found_exception"}}`)
		if code != 200 {
			t.Fatalf("envelope code = %d", code)
		}
		if result["matched"] != false {
			t.Fatalf("matched = %v, want false", result["matched"])
		}
	})
}

func TestCleanupLogDataStreamRejectsBadInput(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"service_id":17,"mode":"weeks"}`,
		`{"service_id":17,"mode":"hours","amount":0}`,
		`{"service_id":17,"mode":"days","amount":99999}`,
	} {
		code, _, _, _ := runCleanup(t, body, http.StatusOK, `{}`)
		if code != 400 {
			t.Fatalf("body %s: envelope code = %d, want 400", body, code)
		}
	}
}

func runCleanup(t *testing.T, requestBody string, upstreamStatus int, upstreamBody string) (int, map[string]any, string, string) {
	t.Helper()
	var capturedPath, capturedBody string
	elasticsearchServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		capturedPath = request.URL.Path + "?" + request.URL.RawQuery
		raw := make([]byte, request.ContentLength)
		_, _ = request.Body.Read(raw)
		capturedBody = string(raw)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(upstreamStatus)
		_, _ = writer.Write([]byte(upstreamBody))
	}))
	defer elasticsearchServer.Close()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sql mock: %v", err)
	}
	defer database.Close()
	mock.ExpectQuery(regexp.QuoteMeta(serviceStreamDimsQuery)).WillReturnRows(sqlmock.NewRows(
		[]string{"project_code", "environment_code", "business_system_code", "service_code"},
	).AddRow("yilake", "poc", "tib", "mgmt"))
	mock.ExpectQuery(`FROM monitor_elasticsearch_cluster`).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "hosts", "username", "password", "verify_tls", "ca_cert", "index_prefix", "request_timeout", "enabled"},
	).AddRow(int64(2), elasticsearchServer.URL, "", "", false, "", "autoadmin", 5, true))

	secrets, err := assets.NewSecretEncryptor("", "test-secret")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database, secrets: secrets}
	engine.POST("/monitor/log-datastreams/cleanup/", handler.CleanupLogDataStream)

	request := httptest.NewRequest(http.MethodPost, "/monitor/log-datastreams/cleanup/", strings.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	var envelope struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
	return envelope.Code, envelope.Data, capturedPath, capturedBody
}
