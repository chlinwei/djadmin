package inspection

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

const groupValidationQuery = `SELECT g.enabled,g.category,(SELECT COUNT(*) FROM inspection_check c WHERE c.group_id=g.id AND c.enabled=TRUE) FROM inspection_group g WHERE g.id=?`

const taskNameScopeQuery = `SELECT COUNT(*) FROM inspection_task WHERE name=? AND group_id=? AND id<>?`

// 挂载点绑定（bindings 输入）必须走挂载校验路径，而不是回落到静态范围校验。
func TestValidateTaskAcceptsServiceMountBinding(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	group := int64(2)
	serviceID := int64(9)
	state := taskState{
		Name: "task", InspectionName: "inspection", Concurrency: 10, TimeoutSeconds: 60,
		Bindings: []groupBindingInput{{
			Group: &group, MountType: mountService, ServiceID: &serviceID, InstanceMode: instanceAll,
		}},
	}
	// 同组内任务名唯一校验在挂载校验之前执行。
	mock.ExpectQuery(regexp.QuoteMeta(taskNameScopeQuery)).WithArgs("task", group, int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta(groupValidationQuery)).WithArgs(group).
		WillReturnRows(sqlmock.NewRows([]string{"enabled", "category", "check_count"}).AddRow(true, "application", 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM assets_application_service WHERE id=?`)).
		WithArgs(serviceID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	message, validationErr := (&Handler{db: database}).validateTask(testGinContext(), &state, true, 0)
	if validationErr != nil || message != "" {
		t.Fatalf("validation result = %q, %v", message, validationErr)
	}
	if len(state.GroupIDs) != 1 || state.GroupIDs[0] != group {
		t.Fatalf("GroupIDs = %v, want [%d]", state.GroupIDs, group)
	}
	// 单组模型：两个绑定直接拒绝（在 DB 查询前拦截）。
	state.Bindings = append(state.Bindings, groupBindingInput{Group: &group, MountType: mountService, ServiceID: &serviceID, InstanceMode: instanceAll})
	if message, _ = (&Handler{db: database}).validateTask(testGinContext(), &state, false, 0); message == "" {
		t.Fatal("two bindings should be rejected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestValidateTaskRejectsMissingMountType(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	group := int64(2)
	state := taskState{
		Name: "task", InspectionName: "inspection", Concurrency: 10, TimeoutSeconds: 60,
		Bindings: []groupBindingInput{{Group: &group}},
	}
	// 同组内唯一校验在挂载校验之前执行。
	mock.ExpectQuery(regexp.QuoteMeta(taskNameScopeQuery)).WithArgs("task", group, int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// 空 mount_type 在组查询之后拦截。
	mock.ExpectQuery(regexp.QuoteMeta(groupValidationQuery)).WithArgs(group).
		WillReturnRows(sqlmock.NewRows([]string{"enabled", "category", "check_count"}).AddRow(true, "application", 1))
	if message, _ := (&Handler{db: database}).validateTask(testGinContext(), &state, false, 0); message == "" {
		t.Fatal("missing mount_type should be rejected")
	}
}

func testGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(nil)
	return context
}
