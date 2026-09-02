package monitor

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"autoadmin/internal/agent/pb"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// 回归用例：批量删除 Exporter 纳管目标要逐个校验 managed_enabled/install_status，
// 单个失败不能影响其它 id，且结果里要按主机名报告成功/失败原因。
func TestBatchDeleteTargetsReportsPerItemResult(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	// id=1: 主机名查询、校验通过、删除成功。
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT h.instance_name,h.ip FROM monitor_target t JOIN assets_host h ON h.id=t.host_id WHERE t.id=?`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"instance_name", "ip"}).AddRow("host-a", "10.0.0.1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT managed_enabled,install_status FROM monitor_target WHERE id=?`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"managed_enabled", "install_status"}).AddRow(false, "success"))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM monitor_target WHERE id=?`)).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// id=2: 仍启用中，必须拒绝删除并原样保留记录。
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT h.instance_name,h.ip FROM monitor_target t JOIN assets_host h ON h.id=t.host_id WHERE t.id=?`)).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"instance_name", "ip"}).AddRow("host-b", "10.0.0.2"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT managed_enabled,install_status FROM monitor_target WHERE id=?`)).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"managed_enabled", "install_status"}).AddRow(true, "success"))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database}
	engine.POST("/targets/batch-delete/", handler.BatchDeleteTargets)

	request := httptest.NewRequest(http.MethodPost, "/targets/batch-delete/", strings.NewReader(`{"ids":[1,2]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"success":1`) || !strings.Contains(body, `"failed":1`) {
		t.Fatalf("body = %s, want success=1 failed=1", body)
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
	failed := &pb.AutomationExecuteResponse{Status: "failed", ExitCode: 1, ErrorMessage: "unsupported action"}
	if _, err := serviceControlResult(failed, "stop", "node_exporter"); err == nil {
		t.Fatalf("want error for failed agent result")
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
