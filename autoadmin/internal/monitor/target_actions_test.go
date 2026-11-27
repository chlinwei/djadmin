package monitor

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

func TestBatchCreateTargetKeepsCommittedIDOnOfflineGuard(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("GetExporterPackageDefaultPort").WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(9100))
	mock.ExpectQuery("GetHostTargetIdentity").WillReturnRows(sqlmock.NewRows([]string{"name", "ip", "deleted"}).AddRow("node-8", "", false))
	expectCreatedMonitorTarget(mock, 12)
	mock.ExpectExec("SetTargetInstallState").WillReturnResult(sqlmock.NewResult(0, 1))
	ctx, rec := monitorBatchRequest(`{"host_ids":[8],"exporter_type":"node_exporter","install_now":true}`)
	(&Handler{db: pool}).BatchCreateTargets(ctx)
	var body struct {
		Code int
		Data struct {
			Count   int
			Results []struct {
				ID            int
				HostID        int `json:"host_id"`
				Host, Message string
				Detail        struct {
					Created  bool
					TargetID int   `json:"target_id"`
					Accepted *bool `json:"install_accepted"`
				}
			}
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || body.Code != 400 || body.Data.Count != 0 || len(body.Data.Results) != 1 {
		t.Fatal(rec.Body.String())
	}
	item := body.Data.Results[0]
	if item.ID != 8 || item.HostID != 8 || item.Host != "node-8" || !strings.Contains(item.Message, "离线") ||
		!item.Detail.Created || item.Detail.TargetID != 12 || item.Detail.Accepted == nil || *item.Detail.Accepted {
		t.Fatal(rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func monitorBatchRequest(jsonBody string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(jsonBody))
	return ctx, rec
}

func TestUpdateTargetMergesLockedState(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, rec := monitorBatchRequest(`{"remark":null}`)
	ctx.Params = gin.Params{{Key: "id", Value: "8"}}
	mock.ExpectBegin()
	mock.ExpectQuery("GetMonitorTargetPatchForUpdate .*FOR UPDATE").WithArgs(8).
		WillReturnRows(sqlmock.NewRows([]string{"exporter_type", "scrape_port", "managed_enabled", "labels", "remark"}).
			AddRow("node_exporter", 9200, true, []byte("{}"), "old"))
	mock.ExpectExec("UpdateMonitorTargetPatch").WithArgs("node_exporter", 9200, true, []byte("{}"), nil, sqlmock.AnyArg(), 8).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	now := time.Now()
	mock.ExpectQuery("GetMonitorTarget :one").WithArgs(8).WillReturnRows(sqlmock.NewRows(strings.Fields(
		"id create_time update_time remark exporter_type managed_enabled install_status install_message last_scrape_status last_scrape_at labels host_id retry_count last_dispatch_manual scrape_port target_type host_name host_ip host_agent_online")).
		AddRow(8, now, now, nil, "node_exporter", true, "success", "", "unknown", nil, []byte("{}"), 1, 0, false, 9200, "exporter", nil, nil, false))
	(&Handler{db: pool}).UpdateTarget(ctx)
	if err := mock.ExpectationsWereMet(); err != nil || !strings.Contains(rec.Body.String(), `"code":200`) {
		t.Fatalf("response=%s expectations=%v", rec.Body.String(), err)
	}
}

func TestTargetInputAndDatabaseErrors(t *testing.T) {
	dbErr := errors.New("database connection detail")
	for _, tt := range []struct {
		name, input string
		update      bool
		err         error
		want        int
	}{
		{"unknown field", `{"scrape_prt":9200}`, true, nil, 400},
		{"write failure", `{"scrape_port":9200}`, true, dbErr, 600},
		{"no enabled package", `{"host_ids":[1],"exporter_type":"node_exporter"}`, false, sql.ErrNoRows, 400},
		{"package query failure", `{"host_ids":[1],"exporter_type":"node_exporter"}`, false, dbErr, 600},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			ctx, rec := monitorBatchRequest(tt.input)
			handler := &Handler{db: pool}
			if tt.update {
				ctx.Params = gin.Params{{Key: "id", Value: "8"}}
				mock.ExpectBegin()
				mock.ExpectQuery("GetMonitorTargetPatchForUpdate").WillReturnRows(sqlmock.NewRows(strings.Fields(
					"exporter_type scrape_port managed_enabled labels remark")).AddRow("node_exporter", 9100, true, []byte("{}"), nil))
				if tt.err != nil {
					mock.ExpectExec("UpdateMonitorTargetPatch").WillReturnError(tt.err)
				}
				mock.ExpectRollback()
				handler.UpdateTarget(ctx)
			} else {
				mock.ExpectQuery("GetExporterPackageDefaultPort").WillReturnError(tt.err)
				handler.BatchCreateTargets(ctx)
			}
			var body struct{ Code int }
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != tt.want || strings.Contains(rec.Body.String(), dbErr.Error()) {
				t.Fatalf("response=%s err=%v", rec.Body.String(), err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCancelTargetRechecksTerminalStateUnderLock(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, rec := monitorBatchRequest("")
	ctx.Params = gin.Params{{Key: "id", Value: "8"}}
	mock.ExpectBegin()
	mock.ExpectQuery("GetLatestTargetInstallHistory .*FOR UPDATE").WithArgs(8).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "start_time"}).AddRow(1, "success", nil))
	mock.ExpectRollback()
	(&Handler{db: pool}).CancelTarget(ctx)
	if err := mock.ExpectationsWereMet(); err != nil || !strings.Contains(rec.Body.String(), `"code":400`) {
		t.Fatalf("response=%s expectations=%v", rec.Body.String(), err)
	}
}

func TestSingleTargetServicePreservesNotFound(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, rec := monitorBatchRequest("")
	ctx.Params = gin.Params{{Key: "id", Value: "9"}}
	mock.ExpectQuery("GetTargetServiceContext").WithArgs(9).WillReturnError(sql.ErrNoRows)
	(&Handler{db: pool}).StopTargetService(ctx)
	if err := mock.ExpectationsWereMet(); err != nil || rec.Code != 200 || !strings.Contains(rec.Body.String(), `"code":404`) {
		t.Fatalf("response=%s expectations=%v", rec.Body.String(), err)
	}
}

func TestBatchMonitorServiceReportsMissingAndOffline(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	handler := &Handler{db: pool}
	for _, run := range []gin.HandlerFunc{handler.BatchStartTargetService, handler.BatchStopTargetService} {
		mock.ExpectQuery("GetTargetHostAddress").WithArgs(int64(8)).WillReturnRows(sqlmock.NewRows([]string{"name", "ip"}).AddRow("node-8", ""))
		mock.ExpectQuery("GetTargetServiceContext").WithArgs(int64(8)).WillReturnRows(sqlmock.NewRows([]string{"name", "exporter"}).AddRow("node-8", "node_exporter"))
		mock.ExpectQuery("GetTargetHostAddress").WithArgs(int64(9)).WillReturnError(sql.ErrNoRows)
		mock.ExpectQuery("GetTargetServiceContext").WithArgs(int64(9)).WillReturnError(sql.ErrNoRows)
		ctx, rec := monitorBatchRequest(`{"ids":[8,9]}`)
		run(ctx)
		var body struct {
			Code int
			Data struct {
				Results []struct {
					ID, Code      int
					Host, Message string
				}
			}
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		items := body.Data.Results
		if rec.Code != 200 || body.Code != 10002 || len(items) != 2 || items[0].ID != 8 || items[0].Host != "node-8" ||
			items[0].Code != 400 || items[0].Message != "host agent is offline" || items[1].ID != 9 || items[1].Code != 404 || items[1].Host != "target-9" {
			t.Fatal(rec.Body.String())
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// 回归用例：批量删除 Exporter 纳管目标要逐个校验 managed_enabled/install_status，
// 单个失败不能影响其它 id，且结果里要按主机名报告成功/失败原因。
func TestBatchDeleteTargetsReportsPerItemResult(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	// id=1: 主机名查询、校验通过、删除成功。
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT h.instance_name, h.ip`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"instance_name", "ip"}).AddRow("host-a", "10.0.0.1"))
	const guardedDelete = `DELETE FROM monitor_target WHERE id = .* AND managed_enabled = FALSE AND install_status <> 'pending'`
	mock.ExpectExec(guardedDelete).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// id=2: 仍启用中，必须拒绝删除并原样保留记录。
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT h.instance_name, h.ip`)).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"instance_name", "ip"}).AddRow("host-b", "10.0.0.2"))
	mock.ExpectExec(guardedDelete).WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT managed_enabled, install_status FROM monitor_target`)).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"managed_enabled", "install_status"}).AddRow(true, "success"))
	mock.ExpectQuery("GetTargetHostAddress").WithArgs(3).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(guardedDelete).WithArgs(3).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("GetMonitorTargetState").WithArgs(3).WillReturnError(sql.ErrNoRows)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database}
	engine.POST("/targets/batch-delete/", handler.BatchDeleteTargets)

	request := httptest.NewRequest(http.MethodPost, "/targets/batch-delete/", strings.NewReader(`{"ids":[1,2,3]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"code":10001`) || !strings.Contains(body, `"code":400`) {
		t.Fatalf("body = %s, want partial failure and per-item business code", body)
	}
	if !strings.Contains(body, `"success":1`) || !strings.Contains(body, `"failed":2`) || !strings.Contains(body, `"code":404`) {
		t.Fatalf("body = %s, want success=1 failed=2 including missing target", body)
	}
	if !strings.Contains(body, "disable the monitor target before deleting it") {
		t.Fatalf("body = %s, want the still-enabled target to be rejected with a reason", body)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestBatchDeleteTargetsRejectsEmptyIDs(t *testing.T) {
	database, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database}
	engine.POST("/targets/batch-delete/", handler.BatchDeleteTargets)

	request := httptest.NewRequest(http.MethodPost, "/targets/batch-delete/", strings.NewReader(`{"ids":[]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if !strings.Contains(recorder.Body.String(), "ids must be a non-empty array") {
		t.Fatalf("body = %s, want a validation error for empty ids", recorder.Body.String())
	}
}

// 回归用例：之前误下发 agent 不认识的 run_shell 动作，agent 返回 failed 却被当成功上报，
// 导致"批量停止提示成功但服务实际没停"。现在必须用 agent 内置的 exporter 动作，
// 并且 agent 报失败（status=failed / exit_code!=0 / 空状态）时要转成错误。
// 空状态是 agent executor 直接报错（版本过旧不认识动作）时的零值结果，最容易漏。
func TestServiceControlResultTreatsAgentFailureAsError(t *testing.T) {
	failed := &pb.AutomationExecuteResponse{JobId: "agent-job-7", Status: "failed", ExitCode: 1, ErrorMessage: "unsupported action"}
	if detail, err := serviceControlResult(failed, "stop", "node_exporter"); err == nil || detail["job_id"] != failed.JobId || !strings.Contains(err.Error(), failed.ErrorMessage) {
		t.Fatalf("want agent job ID and execution reason, got %v %v", detail, err)
	}

	nonZeroExit := &pb.AutomationExecuteResponse{Status: "success", ExitCode: 5, Stderr: "Failed to stop node_exporter.service"}
	if _, err := serviceControlResult(nonZeroExit, "stop", "node_exporter"); err == nil || !strings.Contains(err.Error(), "node_exporter.service") {
		t.Fatalf("want error with service context, got %v", err)
	}

	emptyStatus := &pb.AutomationExecuteResponse{Status: "", ExitCode: 0, ErrorMessage: `unsupported action "stop_exporter"`}
	if _, err := serviceControlResult(emptyStatus, "stop", "node_exporter"); err == nil || !strings.Contains(err.Error(), "unsupported action") {
		t.Fatalf("want error for empty-status result, got %v", err)
	}

	success := &pb.AutomationExecuteResponse{Status: "success", ExitCode: 0, Stdout: ""}
	detail, err := serviceControlResult(success, "stop", "node_exporter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail["exit_code"] != int32(0) {
		t.Fatalf("detail = %v, want exit_code 0", detail)
	}

	// status 查询是只读探测：systemctl status 在服务 inactive/failed 时返回非零退出码，
	// 这是有效状态信息不是错误，必须原样透传给前端判断。
	inactiveStatus := &pb.AutomationExecuteResponse{Status: "failed", ExitCode: 3, Stdout: "inactive (dead)"}
	detail, err = serviceControlResult(inactiveStatus, "status", "node_exporter")
	if err != nil {
		t.Fatalf("status passthrough should not error, got %v", err)
	}
	if detail["exit_code"] != int32(3) {
		t.Fatalf("status detail = %v, want exit_code 3", detail)
	}
}
