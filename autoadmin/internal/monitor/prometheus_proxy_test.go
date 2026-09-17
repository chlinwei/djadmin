package monitor

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func newPrometheusProxyHandler(t *testing.T, upstream http.HandlerFunc) (*Handler, *httptest.Server, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	upstreamServer := httptest.NewServer(upstream)
	t.Cleanup(upstreamServer.Close)

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	mock.ExpectQuery("SELECT value FROM sys_config").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(upstreamServer.URL))

	return &Handler{db: database, client: upstreamServer.Client()}, upstreamServer, mock
}

// assertConfigQueryConsumed 断言 base URL 查询确实命中了 mock。
//
// 这条断言针对的是"静默退化"（P4-8）：改了查询文本后 sqlmock 的片断不再匹配，
// prometheusBaseURL 吞掉错误回退到默认地址，用例于是悄悄去打真实 Prometheus——
// 而响应里恰好也有同名指标，状态码断言照样通过。ExpectationsWereMet 能挡住
// "期望未被消费"（未匹配就是未消费）。
func assertConfigQueryConsumed(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sys_config 查询未命中 mock（查询文本可能已变，用例退化去打真实上游）：%v", err)
	}
}

func performProxyRequest(t *testing.T, handler *Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.Any("/monitor/targets/prometheus/proxy/*apiPath", handler.PrometheusProxy)
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestPrometheusProxyLabelValuesArray(t *testing.T) {
	handler, _, mock := newPrometheusProxyHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/label/__name__/values" {
			t.Errorf("unexpected upstream path %q", r.URL.Path)
		}
		fmt.Fprint(w, `{"status":"success","data":["go_goroutines","up"]}`)
	})

	recorder := performProxyRequest(t, handler, http.MethodGet,
		"/monitor/targets/prometheus/proxy/api/v1/label/__name__/values", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	got := recorder.Body.String()
	if !strings.Contains(got, `"status":"success"`) || !strings.Contains(got, `"go_goroutines"`) {
		t.Fatalf("array data not passed through: %s", got)
	}
	assertConfigQueryConsumed(t, mock)
}

func TestPrometheusProxyPostForm(t *testing.T) {
	handler, _, mock := newPrometheusProxyHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("upstream method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/labels" {
			t.Errorf("unexpected upstream path %q", r.URL.Path)
		}
		if r.URL.Query().Get("match[]") != `up` || r.URL.Query().Get("limit") != "400" {
			t.Errorf("form params missing in upstream query: %v", r.URL.Query())
		}
		fmt.Fprint(w, `{"status":"success","data":["__name__","job"]}`)
	})

	form := url.Values{"match[]": {`up`}, "limit": {"400"}}
	recorder := performProxyRequest(t, handler, http.MethodPost,
		"/monitor/targets/prometheus/proxy/api/v1/labels", form.Encode())
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"__name__"`) {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
	assertConfigQueryConsumed(t, mock)
}

func TestPrometheusProxyRejectsNonAPIPath(t *testing.T) {
	handler, _, _ := newPrometheusProxyHandler(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("upstream should not be called, got %s", r.URL.Path)
	})
	recorder := performProxyRequest(t, handler, http.MethodGet,
		"/monitor/targets/prometheus/proxy/other/path", "")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestPrometheusProxyUpstreamError(t *testing.T) {
	handler, _, mock := newPrometheusProxyHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"status":"error","errorType":"unavailable","error":"storage out of order"}`)
	})
	recorder := performProxyRequest(t, handler, http.MethodGet,
		"/monitor/targets/prometheus/proxy/api/v1/query?query=up", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with error payload", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"status":"error"`) || !strings.Contains(body, "storage out of order") {
		t.Fatalf("error not propagated: %s", body)
	}
	assertConfigQueryConsumed(t, mock)
}

func TestPrometheusQueryDataMap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = database.Close() }()
	// 基础地址查询失败时回退默认值，这里直接让 mock 报错验证回退路径
	mock.ExpectQuery("SELECT value FROM sys_config").WillReturnError(errors.New("no config"))
	handler := &Handler{db: database, client: http.DefaultClient}
	if got := handler.prometheusBaseURL(t.Context()); got != defaultPrometheusBaseURL {
		t.Fatalf("prometheusBaseURL = %q, want default %q", got, defaultPrometheusBaseURL)
	}
}
