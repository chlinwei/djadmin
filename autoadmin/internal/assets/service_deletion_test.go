package assets

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func expectServiceSaveFence(mock sqlmock.Sqlmock, id int64) {
	mock.ExpectQuery("SELECT id FROM assets_application_service WHERE id=.*FOR UPDATE").
		WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
	mock.ExpectQuery("SELECT id FROM assets_business_system WHERE").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("FROM assets_service_deletion WHERE service_id=").
		WithArgs(id).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM assets_service_deletion").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
}

func TestFinishServiceDeletionRemovesOnlyUnsharedDeployments(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM assets_application_service").
		WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery("SELECT d.id FROM assets_application_deployment").
		WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11).AddRow(12))
	mock.ExpectExec("DELETE FROM assets_application_service_log_setting").WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM assets_application_service_deployment").WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	for _, id := range []int64{11, 12} {
		affected := int64(1)
		if id == 12 {
			affected = 0 // Another service still references this deployment.
		}
		mock.ExpectExec("DELETE FROM assets_application_deployment[\\s\\S]+NOT EXISTS").
			WithArgs(id).WillReturnResult(sqlmock.NewResult(0, affected))
	}
	mock.ExpectExec("DELETE FROM assets_application_service WHERE").WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err = NewRepository(pool).finishServiceDeletion(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type failedServiceLogDeleter struct{}

func (failedServiceLogDeleter) RevokeServiceLogs(context.Context, int64) error {
	return errors.New("ES unavailable")
}

func TestServiceDeletionFailureKeepsServiceForRetry(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("commits ownership before external cleanup", func(t *testing.T) {
		pool, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id FROM assets_application_service").
			WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
		mock.ExpectQuery("SELECT business_system_id FROM assets_application_service").
			WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"business_system_id"}).AddRow(1))
		mock.ExpectQuery("SELECT id FROM assets_business_system").
			WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		mock.ExpectQuery("FROM assets_service_deletion WHERE service_id=").
			WithArgs(int64(7)).WillReturnError(sql.ErrNoRows)
		mock.ExpectQuery("SELECT p.code AS project_code").
			WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{
			"project_code", "environment_code", "business_system_code", "service_code",
		}).AddRow("p", "prod", "b", "SVC"))
		mock.ExpectExec("INSERT INTO assets_service_deletion").
			WithArgs(int64(7), "p", "b", "prod", "SVC").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("UPDATE assets_application_service").
			WithArgs(sqlmock.AnyArg(), false, int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		exists, err := NewRepository(pool).beginServiceDeletion(context.Background(), 7)
		if err != nil || !exists {
			t.Fatalf("begin deletion: %v %v", exists, err)
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	defer pool.Close()
	service := newTestService(t, NewRepository(pool))
	service.SetServiceLogDeleter(failedServiceLogDeleter{})
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM assets_application_service").
		WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery("SELECT business_system_id FROM assets_application_service").
		WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"business_system_id"}).AddRow(1))
	mock.ExpectQuery("SELECT id FROM assets_business_system").
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("FROM assets_service_deletion WHERE service_id=").
		WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{
		"service_id", "project_code", "business_system_code", "environment_code", "service_code",
	}).AddRow(7, "p", "b", "prod", "svc"))
	mock.ExpectExec("UPDATE assets_application_service").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err = service.DeleteApplicationService(context.Background(), 7); err == nil {
		t.Fatal("external cleanup failure must not report success")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
