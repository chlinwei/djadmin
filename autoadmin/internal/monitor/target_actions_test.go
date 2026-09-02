package monitor

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

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
