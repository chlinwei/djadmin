package monitor

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNotificationPolicySelectedNames(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("ListNotificationPolicyNodes").WillReturnRows(policyTreeRow(1, 0, "根", "[3,4]", `[{"type":"tree","node_type":"environment","id":7}]`))
	mock.ExpectQuery("GetBusinessEnvironmentNameByID").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("预生产"))
	mock.ExpectQuery("GetAlertMediaName").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"name"}))
	mock.ExpectQuery("GetAlertMediaName").WithArgs(int64(4)).WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("夜间邮件"))
	rows, err := (&Handler{db: pool}).listNotificationPolicies(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ScopeNames["environment:7"] != "预生产" ||
		len(rows[0].MediaNames) != 2 || rows[0].MediaNames[0] != "媒介 #3（已删除）" || rows[0].MediaNames[1] != "夜间邮件" {
		t.Fatalf("selected names: %+v", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
