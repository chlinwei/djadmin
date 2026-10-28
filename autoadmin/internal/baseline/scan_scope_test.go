package baseline

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestLoadScanScope(t *testing.T) {
	for _, selected := range []bool{false, true} {
		t.Run(map[bool]string{false: "all environments", true: "selected environment"}[selected], func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			environmentID := sql.NullInt64{}
			if selected {
				environmentID = sql.NullInt64{Int64: 2, Valid: true}
			}
			mock.ExpectQuery("SELECT DISTINCT d.host_id, s.environment_id").
				WithArgs(int64(7), environmentID, environmentID).
				WillReturnRows(sqlmock.NewRows([]string{"host_id", "environment_id", "environment_name"}).
					AddRow(10, 1, "test").AddRow(10, 2, "prod").AddRow(11, 2, "prod").
					AddRow(99, 3, "not-scanned").AddRow(12, nil, ""))
			scope, err := loadScanScope(context.Background(), db.New(pool), sql.NullInt64{Int64: 7, Valid: true}, environmentID, "project", "prod", []int64{10, 11, 12})
			if err != nil {
				t.Fatal(err)
			}
			if selected {
				if len(scope.Environments) != 1 || scope.Environments[0].ID != 2 || len(scope.Hosts[12]) != 1 {
					t.Fatalf("selected scope: %+v", scope)
				}
			} else if len(scope.Environments) != 2 || scope.Environments[0].Name != "test" ||
				len(scope.Hosts[10]) != 2 || len(scope.Hosts[11]) != 1 || len(scope.Hosts[12]) != 0 {
				t.Fatalf("all scope: %+v", scope)
			}
			restored, err := decodeScanScope(jsonBytes(scope))
			if err != nil || restored.ProjectName != "project" || len(restored.Hosts) != 3 {
				t.Fatalf("snapshot roundtrip: %+v, %v", restored, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLoadScanScopeReportsQueryFailure(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("SELECT DISTINCT d.host_id, s.environment_id").WillReturnError(sql.ErrConnDone)
	_, err = loadScanScope(context.Background(), db.New(pool), sql.NullInt64{Int64: 1, Valid: true}, sql.NullInt64{}, "", "", []int64{1})
	if err != sql.ErrConnDone {
		t.Fatalf("error hidden: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeScanScopeRejectsInvalidSnapshot(t *testing.T) {
	for _, raw := range []string{"{", "null", "{}", `{"environments":[]}`} {
		if _, err := decodeScanScope([]byte(raw)); err == nil {
			t.Fatalf("invalid snapshot accepted: %s", raw)
		}
	}
}

func TestGetScanLegacyScopeIsExplicitlyCurrent(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("FROM security_scan s JOIN baseline b").WithArgs(int64(28)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "scan_type", "baseline", "baseline_id", "mount_type", "status",
			"summary", "requested_username", "start_time", "end_time", "project_id", "environment_id", "project_name", "environment_name", "scope_snapshot", "scope_snapshot_present"}).
			AddRow(28, "baseline", "baseline", 7, "project", "failed", []byte(`{}`), "admin", nil, nil, 3, nil, "pvg4", "", []byte(`null`), 0))
	mock.ExpectQuery("FROM security_scan_target").WithArgs(int64(28)).
		WillReturnRows(sqlmock.NewRows([]string{"host_id", "host_name", "host_ip", "status", "passed_items", "failed_items", "compliance_rate", "error_message"}).
			AddRow(10, "host", "10.0.0.1", "failed", 1, 1, "50.00", ""))
	mock.ExpectQuery("SELECT DISTINCT d.host_id, s.environment_id").WithArgs(int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"host_id", "environment_id", "environment_name"}).
			AddRow(10, 1, "test").AddRow(10, 2, "prod").AddRow(99, 3, "not-scanned"))
	mock.ExpectQuery("FROM baseline_scan_result r").WithArgs(int64(28)).
		WillReturnRows(sqlmock.NewRows([]string{"host_id", "host_name", "host_ip", "item_name", "chapter", "severity",
			"status", "expected_value", "actual_value", "message", "remediation"}).
			AddRow(10, "host", "10.0.0.1", "check", "", "low", "fail", []byte(`null`), []byte(`null`), "", ""))
	ctx, recorder := ginTestContext("GET", "/sys/security/scans/28/", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "28"}}
	(&Handler{db: pool}).GetScan(ctx)
	data := decodeResponse(t, recorder)["data"].(map[string]any)
	scan := data["scan"].(map[string]any)
	if scan["project_name"] != "pvg4" || scan["environment_source"] != "current_assets" || len(scan["environments"].([]any)) != 2 {
		t.Fatalf("legacy scope: %v", scan)
	}
	for _, key := range []string{"targets", "items"} {
		row := data[key].([]any)[0].(map[string]any)
		if len(row["environments"].([]any)) != 2 {
			t.Fatalf("%s environments: %v", key, row)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type scopeSnapshotArgument struct{}

func (scopeSnapshotArgument) Match(value driver.Value) bool {
	raw, ok := value.([]byte)
	if !ok {
		return false
	}
	scope, err := decodeScanScope(raw)
	return err == nil && scope.ProjectName == "project" && len(scope.Environments) == 1 &&
		scope.Environments[0].Name == "test" && len(scope.Hosts[10]) == 1
}

func TestStartScanWritesScopeInTargetTransaction(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("SELECT id, name, version, enabled FROM baseline").WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "version", "enabled"}).AddRow(7, "baseline", "1", true))
	mock.ExpectQuery("SELECT COUNT").WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT DISTINCT h.id").WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "instance_name", "ip", "agent_online"}).AddRow(10, "host", "10.0.0.1", true))
	mock.ExpectQuery("SELECT name FROM assets_project").WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("project"))
	mock.ExpectQuery("SELECT DISTINCT d.host_id, s.environment_id").WithArgs(int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"host_id", "environment_id", "environment_name"}).AddRow(10, 1, "test"))
	expectCreateReturnsID(mock, "INSERT INTO security_scan(", []driver.Value{
		sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7), "project", int64(3), nil, "", scopeSnapshotArgument{},
	}, 28)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO security_scan_target(")).
		WillReturnError(errors.New("target insert failed"))
	mock.ExpectRollback()
	ctx, recorder := ginTestContext("POST", "/sys/security/baseline/7/scan/", map[string]any{"mount_type": "project", "project_id": 3})
	(&Handler{db: pool}).StartScan(ctx)
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != float64(600) {
		t.Fatalf("target failure must roll back snapshot and scan: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
