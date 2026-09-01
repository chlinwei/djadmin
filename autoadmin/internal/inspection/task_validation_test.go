package inspection

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

const groupValidationQuery = `SELECT g.scope,g.enabled,(SELECT COUNT(*) FROM inspection_check c WHERE c.group_id=g.id AND c.enabled=TRUE) FROM inspection_group g WHERE g.id=?`

func TestValidateTaskDeduplicatesSelectedHostIDs(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	mock.ExpectQuery(regexp.QuoteMeta(groupValidationQuery)).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"scope", "enabled", "check_count"}).AddRow("per_host", true, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM assets_host WHERE is_deleted_in_cloud=FALSE AND id IN (?,?)`)).
		WithArgs(int64(10), int64(20)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	state := validPerHostTaskState([]int64{10, 10, 20})
	message, validationErr := (&Handler{db: database}).validateTask(testGinContext(), &state, true)
	if validationErr != nil || message != "" {
		t.Fatalf("validation result = %q, %v", message, validationErr)
	}
	if len(state.SelectedHostIDs) != 2 || state.SelectedHostIDs[0] != 10 || state.SelectedHostIDs[1] != 20 {
		t.Fatalf("selected_host_ids = %v, want [10 20]", state.SelectedHostIDs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestValidateTaskPropagatesHostCountError(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	mock.ExpectQuery(regexp.QuoteMeta(groupValidationQuery)).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"scope", "enabled", "check_count"}).AddRow("per_host", true, 1))
	databaseErr := errors.New("database unavailable")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM assets_host WHERE is_deleted_in_cloud=FALSE AND id IN (?)`)).
		WithArgs(int64(10)).WillReturnError(databaseErr)

	state := validPerHostTaskState([]int64{10})
	message, validationErr := (&Handler{db: database}).validateTask(testGinContext(), &state, true)
	if message != "" || !errors.Is(validationErr, databaseErr) {
		t.Fatalf("validation result = %q, %v; want database error", message, validationErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func validPerHostTaskState(hostIDs []int64) taskState {
	return taskState{Name: "task", InspectionName: "inspection", GroupID: 1, SelectedHostIDs: hostIDs, Concurrency: 10, TimeoutSeconds: 60}
}

func testGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(nil)
	return context
}
