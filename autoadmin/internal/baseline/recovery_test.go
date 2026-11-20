package baseline

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBaselineStartupDoesNotReplayUnconfirmedTargets(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery("FROM security_scan WHERE scan_type = 'baseline' AND status IN").
		WillReturnRows(sqlmock.NewRows([]string{"id", "dispatch_snapshot"}).
			AddRow(19, []byte(`{"Items":[{"name":"saved check"}],"Hosts":[{"Host":{"id":7},"InstanceName":"vm-1"}]}`)).
			AddRow(20, []byte(`{}`)))
	for _, row := range []struct {
		id      int64
		missing bool
	}{{19, false}, {20, true}} {
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE security_scan SET status = 'pending' WHERE").
			WithArgs(row.id).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("UPDATE security_scan_target SET status = 'interrupted'.*status = 'running'").
			WithArgs(sqlmock.AnyArg(), row.id, row.missing).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}
	if _, err := (&Handler{db: database}).RecoverDispatches(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
