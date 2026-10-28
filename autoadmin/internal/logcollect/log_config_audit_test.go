package logcollect

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"autoadmin/internal/api/response"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestAuditLogConfigsReportsUnmanagedHostConflict(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("name: ListLogConfigAuditHosts").WillReturnRows(
		sqlmock.NewRows([]string{"host_id", "host_ip", "host_instance_name"}).
			AddRow(42, "10.0.0.42", "host-42").AddRow(43, "10.0.0.43", "host-43"))
	expectServiceConflictInputs(mock, "${APP_HOME}/logs/app.log")
	mock.ExpectCommit()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/monitor/log-targets/config-audit/", nil)
	(&Handler{db: pool}).AuditLogConfigs(c)
	var envelope struct {
		Code int                  `json:"code"`
		Msg  string               `json:"msg"`
		Data logConfigAuditResult `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	result := envelope.Data
	if recorder.Code != http.StatusOK || envelope.Code != 200 || envelope.Msg != "success" ||
		result.HostsChecked != 2 || result.HostsWithErrors != 1 || result.ProblemCount != 1 || len(result.Results) != 1 {
		t.Fatalf("unexpected audit response: %s", recorder.Body.String())
	}
	host := result.Results[0]
	if host.HostID != 42 || host.HostIP != "10.0.0.42" || host.HostInstanceName != "host-42" ||
		!strings.Contains(strings.Join(host.Errors, ";"), "/data/app/logs/app.log") {
		t.Fatalf("audit must identify conflicting unmanaged host: %+v", host)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAuditLogConfigsEmptyResultIsArray(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("name: ListLogConfigAuditHosts").WillReturnRows(
		sqlmock.NewRows([]string{"host_id", "host_ip", "host_instance_name"}))
	mock.ExpectCommit()
	result, err := (&Handler{db: pool}).auditLogConfigs(context.Background())
	if err != nil || result.HostsChecked != 0 || result.Results == nil || len(result.Results) != 0 {
		t.Fatalf("empty database must report zero checked hosts with []: %+v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAuditLogConfigsBatchesInOneSnapshot(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	hosts := sqlmock.NewRows([]string{"host_id", "host_ip", "host_instance_name"})
	for id := 1; id <= 201; id++ {
		hosts.AddRow(id, "", "")
	}
	mock.ExpectQuery("name: ListLogConfigAuditHosts").WillReturnRows(hosts)
	expectServiceConflictInputs(mock, "/data/app/logs/other.log")
	mock.ExpectQuery("FROM assets_application_service_deployment").WillReturnRows(
		sqlmock.NewRows([]string{"host_id", "service_id", "service_code", "instance_name",
			"runtime_variables", "app_home", "host_ip"}))
	mock.ExpectQuery("FROM assets_application_service s").WillReturnRows(sqlmock.NewRows([]string{"host_id"}))
	mock.ExpectCommit()
	result, err := (&Handler{db: pool}).auditLogConfigs(context.Background())
	if err != nil || result.HostsChecked != 201 || result.HostsWithErrors != 0 || len(result.Results) != 0 {
		t.Fatalf("batch audit failed: %+v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAuditLogConfigsDoesNotReturnPartialSuccess(t *testing.T) {
	for _, stage := range []string{"begin", "hosts", "inputs", "commit"} {
		t.Run(stage, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			failure := errors.New("audit " + stage + " failed")
			if stage == "begin" {
				mock.ExpectBegin().WillReturnError(failure)
			} else {
				mock.ExpectBegin()
				query := mock.ExpectQuery("name: ListLogConfigAuditHosts")
				if stage == "hosts" {
					query.WillReturnError(failure)
					mock.ExpectRollback()
				} else {
					query.WillReturnRows(sqlmock.NewRows([]string{"host_id", "host_ip", "host_instance_name"}).
						AddRow(42, "", ""))
					if stage == "inputs" {
						mock.ExpectQuery("FROM assets_application_service_deployment").WillReturnError(failure)
						mock.ExpectRollback()
					} else {
						expectServiceConflictInputs(mock, "${APP_HOME}/logs/app.log")
						mock.ExpectCommit().WillReturnError(failure)
					}
				}
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/monitor/log-targets/config-audit/", nil)
			(&Handler{db: pool}).AuditLogConfigs(c)
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Code == 200 || envelope.Data != nil {
				t.Fatalf("incomplete audit must fail without a success-shaped partial result: %s", recorder.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
