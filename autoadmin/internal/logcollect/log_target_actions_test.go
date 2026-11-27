package logcollect

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/agent/pb"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestBatchLogServiceReportsAgentExecutionFailure(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	handler := &Handler{db: pool, gateway: startTestAgent(t, func(frame *pb.ServerFrame) *pb.AgentFrame {
		req := frame.GetAutomationExecuteRequest()
		if req == nil {
			return nil
		}
		if strings.Contains(req.ParamsJson, `"stop"`) {
			return testPathResponse(req, "failed", "", "unit refused to stop")
		}
		return testPathResponse(req, "success", "", "")
	})}
	for _, action := range []string{"start", "stop"} {
		mock.ExpectQuery("GetLogTargetForAction").WithArgs(int64(7)).WillReturnRows(
			sqlmock.NewRows([]string{"id", "host", "enabled", "status", "name", "ip", "os", "like", "version"}).AddRow(7, 4, true, "success", "host-01", "", "", "", ""))
		wantCode := 400
		if action == "start" {
			wantCode = 200
			mock.ExpectExec("SetLogTargetRuntimeStatus").WillReturnResult(sqlmock.NewResult(0, 1))
		}
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"ids":[7]}`))
		if action == "start" {
			handler.BatchStartLogTargets(ctx)
		} else {
			handler.BatchStopLogTargets(ctx)
		}
		var body struct {
			Code int
			Data struct {
				Results []struct {
					ID, Code      int
					Host, Message string
					Detail        struct {
						JobID string `json:"job_id"`
					}
				}
			}
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if rec.Code != 200 || body.Code != wantCode || len(body.Data.Results) != 1 {
			t.Fatal(rec.Body.String())
		}
		item := body.Data.Results[0]
		if item.ID != 7 || item.Code != wantCode || item.Host != "host-01" || item.Detail.JobID == "" ||
			(action == "stop" && !strings.Contains(item.Message, "unit refused to stop")) {
			t.Fatal(rec.Body.String())
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBatchCreateLogTargetsReportsInstallAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name            string
		publishErr      error
		wantCode, count int
	}{
		{"accepted and missing host", nil, 10001, 1},
		{"publish failed after creating target and job", errors.New("broker details"), 10002, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			publisher := &fakeLogBatchPublisher{err: tc.publishErr}
			handler := &Handler{db: pool, batchRunner: newLogBatchRunner(t, pool, publisher, time.Minute)}
			mock.ExpectQuery("GetHostTargetIdentity").WithArgs(int64(4)).WillReturnRows(
				sqlmock.NewRows([]string{"name", "ip", "deleted"}).AddRow("", "10.0.0.4", false))
			expectCreatedBatchRow(mock, "CreateLogCollectionTargetIfAbsent", 12)
			mock.ExpectQuery("GetHostTargetIdentity").WithArgs(int64(5)).WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery("ListLogBatchTargets").WillReturnRows(sqlmock.NewRows([]string{"id", "host_id", "name", "ip"}).AddRow(12, 4, "", "10.0.0.4"))
			expectCreatedBatchRow(mock, "CreateLogBatchJob :", 1)
			mock.ExpectExec("CreateLogBatchJobItem").WillReturnResult(sqlmock.NewResult(2, 1))
			if tc.publishErr != nil {
				mock.ExpectExec(batchFinishSQL).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"host_ids":[4,5],"install_now":true}`))
			handler.BatchCreateLogTargets(ctx)
			var body struct {
				Code int
				Data struct {
					Count        int
					InstallBatch *struct{ ID int64 } `json:"install_batch"`
					InstallError string              `json:"install_error"`
					Results      []struct {
						ID, Code      int
						HostID        int `json:"host_id"`
						Host, Message string
						OK            bool
						Detail        struct {
							TargetID int `json:"target_id"`
							Created  bool
							JobID    int  `json:"job_id"`
							Accepted bool `json:"install_accepted"`
						}
					}
				}
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if rec.Code != 200 || body.Code != tc.wantCode || body.Data.Count != tc.count || len(body.Data.Results) != 2 {
				t.Fatal(rec.Body.String())
			}
			item, missing := body.Data.Results[0], body.Data.Results[1]
			accepted := tc.publishErr == nil
			if item.OK != accepted || item.HostID != 4 || item.Host != "10.0.0.4" || !item.Detail.Created ||
				item.Detail.TargetID != 12 || item.Detail.JobID != 1 || item.Detail.Accepted != accepted || missing.ID != 5 || missing.Code != 404 ||
				accepted != (body.Data.InstallBatch != nil && body.Data.InstallBatch.ID == 1) ||
				(!accepted && (body.Data.InstallError != "批量作业入队失败" || item.Message != body.Data.InstallError)) {
				t.Fatal(rec.Body.String())
			}
			if len(publisher.messages) != 1 || publisher.messages[0].ResourceID != 1 {
				t.Fatal(publisher.messages)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

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
