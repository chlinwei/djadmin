package logcollect

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func expectNoSharedTargets(mock sqlmock.Sqlmock, serviceID int64) {
	mock.ExpectQuery("FROM monitor_shared_log_source_service sl").WithArgs(serviceID).
		WillReturnRows(sqlmock.NewRows([]string{"host_id"}))
}

func expectNoSharedRevocations(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("FROM monitor_shared_log_source_revocation v").
		WillReturnRows(sqlmock.NewRows([]string{"source_id", "generation", "stopped", "last_error", "host_name"}))
}

func sharedSourceModelRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "host_id", "business_system_id", "environment_id",
		"path_pattern", "processing_rule_id", "retention_tier_id", "include_pattern", "exclude_pattern", "generation", "revision", "archived"}).
		AddRow(5, "MQ", 779, 1, 1, "/var/mqm/errors/AMQERR01.LOG", 2, 3, "", "", 7, 1, false)
}

func TestServiceDeletionDetachesOnlyItsSharedSourceAssociation(t *testing.T) {
	for _, last := range []bool{false, true} {
		t.Run(map[bool]string{false: "other service survives", true: "last association queues stop"}[last], func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			mock.ExpectQuery("SELECT source_id FROM monitor_shared_log_source_service").
				WithArgs(int64(32)).WillReturnRows(sqlmock.NewRows([]string{"source_id"}).AddRow(5))
			mock.ExpectBegin()
			mock.ExpectQuery("FROM monitor_shared_log_source WHERE id=").WithArgs(int64(5)).
				WillReturnRows(sharedSourceModelRows())
			members := sqlmock.NewRows([]string{"service_id"}).AddRow(32)
			if !last {
				members.AddRow(33)
			}
			mock.ExpectQuery("SELECT service_id FROM monitor_shared_log_source_service").WithArgs(int64(5)).
				WillReturnRows(members)
			mock.ExpectExec("DELETE FROM monitor_shared_log_source_service").WithArgs(int64(5), int64(32)).
				WillReturnResult(sqlmock.NewResult(0, 1))
			if last {
				mock.ExpectExec("INSERT INTO monitor_shared_log_source_revocation").WithArgs(int64(5), int64(7)).
					WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE monitor_shared_log_source SET generation=generation\\+1").WithArgs(int64(5)).
					WillReturnResult(sqlmock.NewResult(0, 1))
			} else {
				mock.ExpectExec("UPDATE monitor_shared_log_source SET revision=revision\\+1").WithArgs(int64(5)).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit()
			handler := &Handler{db: pool}
			if err = handler.detachServiceSharedSources(context.Background(), 32); err != nil {
				t.Fatal(err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
