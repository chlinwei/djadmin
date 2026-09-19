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
	return runCleanupWithTiers(t, requestBody, nil, false, upstreamStatus, upstreamBody)
}

// runCleanupWithTiers 比 runCleanup 多一个"档位编码查询"的预期：只有请求里带了 tier 才会查它。
// 没带预期却查了 = mock 报错，等于在测试层面钉住"档位必须先过档位表校验"。
func runCleanupWithTiers(t *testing.T, requestBody string, tierCodes []string, expectTierQuery bool, upstreamStatus int, upstreamBody string) (int, map[string]any, string, string) {
	t.Helper()
	var capturedPath, capturedBody string
	var upstreamCalled bool
	elasticsearchServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalled = true
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
	if expectTierQuery {
		rows := sqlmock.NewRows([]string{"code"})
		for _, code := range tierCodes {
			rows.AddRow(code)
		}
		mock.ExpectQuery(`SELECT code FROM monitor_log_retention_tier`).WillReturnRows(rows)
	}

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
	_ = upstreamCalled
	return envelope.Code, envelope.Data, capturedPath, capturedBody
}

// 按流清理（可选 tier）：把范围从"该服务所有档位"收窄到某一条流，用于回收不打算再切回的
// 历史档位流。档位编码**必须先命中档位表**——它直接进 ES 的索引模式，放行任意字符串就等于
// 重新开出"删任意索引"的口子。
func TestCleanupLogDataStreamNarrowsToSingleTier(t *testing.T) {
	code, result, capturedPath, capturedBody := runCleanupWithTiers(
		t, `{"service_id":17,"mode":"all","tier":"wuhan-test"}`,
		[]string{"hot", "std", "wuhan-test"}, true, http.StatusOK, `{"task":"node:1"}`)
	if code != 200 {
		t.Fatalf("envelope code = %d, data = %v", code, result)
	}
	// 精确到这一条流，不能留通配符（否则会连带清掉其他档位的数据）。
	if !strings.Contains(capturedPath, "/autoadmin-yilake-tib-poc-mgmt-wuhan-test/_delete_by_query") {
		t.Fatalf("upstream path = %q, want 单条流的精确路径", capturedPath)
	}
	if strings.Contains(capturedPath, "*") {
		t.Fatalf("按流清理不该出现通配符：%q", capturedPath)
	}
	if result["stream_pattern"] != "autoadmin-yilake-tib-poc-mgmt-wuhan-test" || result["tier"] != "wuhan-test" {
		t.Fatalf("响应应回精确流名与档位：%v", result)
	}
	if !strings.Contains(capturedBody, `"match_all"`) {
		t.Fatalf("mode=all 应清空该流全部文档，body = %q", capturedBody)
	}
}

func TestCleanupLogDataStreamRejectsUnknownTier(t *testing.T) {
	// 档位表里没有 "bogus"：必须 400，且**不能**碰 Elasticsearch。
	elasticsearchServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Fatalf("未知档位不该发起 ES 请求：%s", request.URL.Path)
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
	mock.ExpectQuery(`SELECT code FROM monitor_log_retention_tier`).WillReturnRows(
		sqlmock.NewRows([]string{"code"}).AddRow("hot").AddRow("std"))

	secrets, err := assets.NewSecretEncryptor("", "test-secret")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/monitor/log-datastreams/cleanup/", (&Handler{db: database, secrets: secrets}).CleanupLogDataStream)

	request := httptest.NewRequest(http.MethodPost, "/monitor/log-datastreams/cleanup/",
		strings.NewReader(`{"service_id":17,"mode":"all","tier":"bogus;DELETE /_all"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	var envelope struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
	if envelope.Code != 400 {
		t.Fatalf("envelope code = %d, want 400（未知档位必须拒绝）", envelope.Code)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}
