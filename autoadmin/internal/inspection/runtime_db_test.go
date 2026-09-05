package inspection

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// 这组测试钉住执行链路的裸 SQL 形状：曾经出现过 INSERT 列占位符错位
// （status 列被写成 trigger_type）导致执行永久卡死、且没有任何报错的问题。
// sqlmock 无法校验 schema（那是 migrate 的职责），但能钉住 SQL 模板与参数顺序，
// 防止手写 SQL 被改坏后无测试兜底。

func TestCreateExecutionInsertShape(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	task := runTask{
		ID: 5, Name: "task", Scope: "service_once", ServiceID: 9, ServiceName: "svc",
		Concurrency: 10, Timeout: 60,
		Groups: []runGroup{{
			ID: 1, Name: "group-a", Scope: "service_once", Category: "general",
			Checks: []runCheck{{Name: "check-1", Executor: "goss", Severity: "critical"}},
		}},
	}
	targets := []runTarget{{Name: "target-1", AgentID: "agent-1"}}

	mock.ExpectBegin()
	// status 恒为字面量 'pending'，trigger_type 才是参数——占位符错位的回归锚点。
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO inspection_execution(task_id,status,trigger_type,")).
		WithArgs(int64(5), "manual", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), int32(7), "ops").
		WillReturnResult(sqlmock.NewResult(201, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO inspection_target_execution(")).
		WithArgs(int64(201), sqlmock.AnyArg(), sqlmock.AnyArg(), "target-1", sqlmock.AnyArg(), sqlmock.AnyArg(), "agent-1").
		WillReturnResult(sqlmock.NewResult(301, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE inspection_task SET last_run_time=NOW()")).
		WithArgs(int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	executionID, err := (&Handler{db: database}).createExecution(context.Background(), task, targets, "manual", 7, "ops")
	if err != nil {
		t.Fatalf("createExecution: %v", err)
	}
	if executionID != 201 {
		t.Fatalf("execution id = %d, want 201", executionID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestCreateExecutionRejectsEmptyGroups(t *testing.T) {
	database, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	// task.Groups 为空说明 prepareRunTask 没装载任何组——这不该走到落库。
	_, err = (&Handler{db: database}).createExecution(context.Background(), runTask{ID: 5}, []runTarget{{Name: "t"}}, "manual", 0, "")
	if err == nil {
		t.Fatal("createExecution with no groups should fail")
	}
}

func TestListExecutionResultsMapsGroupColumns(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	query := regexp.QuoteMeta("SELECT r.id,r.target_id,r.check_key,r.check_type,r.name,r.status,r.severity,r.group_id,r.group_name,r.expected_value,r.actual_value,r.message FROM inspection_result r JOIN inspection_target_execution t ON t.id=r.target_id WHERE t.execution_id=? ORDER BY r.target_id,r.id")
	mock.ExpectQuery(query).WithArgs(int64(201)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "target_id", "check_key", "check_type", "name", "status", "severity", "group_id", "group_name", "expected_value", "actual_value", "message"}).
			AddRow(1, 301, "inspection:201:0", "goss", "check-1", "fail", "critical", 1, "group-a", []byte("null"), []byte("null"), "failed").
			AddRow(2, 301, "inspection:201:1", "goss", "check-2", "pass", "warning", nil, "", []byte("null"), []byte("null"), ""))

	results, err := (&Handler{db: database}).listExecutionResults(context.Background(), 201)
	if err != nil {
		t.Fatalf("listExecutionResults: %v", err)
	}
	if len(results[301]) != 2 {
		t.Fatalf("results for target 301 = %d, want 2", len(results[301]))
	}
	if results[301][0].GroupID == nil || *results[301][0].GroupID != 1 || results[301][0].GroupName != "group-a" {
		t.Fatalf("first result group = %v/%q, want 1/group-a", results[301][0].GroupID, results[301][0].GroupName)
	}
	if results[301][1].GroupID != nil || results[301][1].GroupName != "" {
		t.Fatalf("second result group = %v/%q, want nil/empty", results[301][1].GroupID, results[301][1].GroupName)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestFlattenChecksCarriesGroup(t *testing.T) {
	groups := []runGroup{
		{ID: 1, Name: "general", Checks: []runCheck{{Name: "a"}, {Name: "b"}}},
		{ID: 2, Name: "app", Checks: []runCheck{{Name: "c"}}},
	}
	checks := flattenChecks(groups)
	if len(checks) != 3 {
		t.Fatalf("flattened checks = %d, want 3", len(checks))
	}
	if checks[0].GroupID != 1 || checks[0].Group != "general" || checks[2].GroupID != 2 || checks[2].Group != "app" {
		t.Fatalf("group attribution wrong: %+v", checks)
	}
	// 序列化后要能还原（检查计划/结果回查依赖 JSON 往返）。
	raw, err := json.Marshal(checks[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back runCheck
	if json.Unmarshal(raw, &back) != nil || back.GroupID != 1 {
		t.Fatalf("json round trip lost group: %s", raw)
	}
}

func TestBusinessChainSnapshotOmitsEmptyLevels(t *testing.T) {
	chain := businessChainSnapshot(
		sql.NullInt64{Int64: 3, Valid: true}, sql.NullInt64{Int64: 7, Valid: true}, sql.NullInt64{},
		"proj", "owner-a", "system", "owner-b", "",
	)
	if chain == nil {
		t.Fatal("chain should not be nil")
	}
	if project, ok := chain["project"].(gin.H); !ok || project["name"] != "proj" {
		t.Fatalf("project chain wrong: %v", chain)
	}
	if system, ok := chain["business_system"].(gin.H); !ok || system["owner"] != "owner-b" {
		t.Fatalf("business_system chain wrong: %v", chain)
	}
	if _, exists := chain["environment"]; exists {
		t.Fatal("empty environment level should be omitted")
	}
	if businessChainSnapshot(sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{}, "", "", "", "", "") != nil {
		t.Fatal("fully empty chain should be nil")
	}
}
