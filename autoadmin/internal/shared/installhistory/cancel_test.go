package installhistory

import (
	"context"
	"errors"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestCancelTransitions(t *testing.T) {
	for _, tc := range []struct {
		name         string
		source       Source
		targetUpdate string
		failWrite    bool
		stale        bool
	}{
		{"exporter", Exporter, "CancelMonitorTargetInstallState", false, false},
		{"filebeat", Filebeat, "CancelLogTargetInstallState", false, false},
		{"history resolves filebeat", History, "CancelLogTargetInstallState", false, false},
		{"filebeat rolls back both writes", Filebeat, "CancelLogTargetInstallState", true, false},
		{"older history cannot reset target", History, "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			mock.ExpectBegin()
			id := int64(8)
			if tc.source == History {
				id = 17
				mock.ExpectQuery("GetInstallHistoryForUpdate .*FOR UPDATE").WithArgs(id).WillReturnRows(
					sqlmock.NewRows([]string{"status", "target_id", "log_collection_target_id", "start_time"}).AddRow("pending", nil, 8, nil))
			}
			latest := 17
			if tc.stale {
				latest++
			}
			lookup := "GetLatestLogTargetInstallHistoryForUpdate"
			if tc.source == Exporter {
				lookup = "GetLatestTargetInstallHistory"
			}
			mock.ExpectQuery(lookup + " .*FOR UPDATE").WithArgs(8).WillReturnRows(
				sqlmock.NewRows([]string{"id", "status", "time"}).AddRow(latest, "running", time.Now()))
			writeErr := errors.New("target write failed")
			if !tc.stale {
				mock.ExpectExec("CancelInstallHistory .*status='cancelled'").WillReturnResult(sqlmock.NewResult(0, 1))
				write := mock.ExpectExec(tc.targetUpdate + " .*install_status='unknown'")
				if tc.failWrite {
					write.WillReturnError(writeErr)
				} else {
					write.WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
			if tc.failWrite || tc.stale {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			err = Cancel(context.Background(), pool, tc.source, id)
			switch {
			case tc.stale:
				if app, ok := apperror.As(err); !ok || app.Code() != 400 {
					t.Fatalf("stale history error = %v", err)
				}
			case tc.failWrite:
				if !errors.Is(err, writeErr) {
					t.Fatalf("target write error = %v", err)
				}
			case err != nil:
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFinishIgnoresLateJob(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	mock.ExpectExec("FinishTargetInstallHistory .*status='pending'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	committed, err := Finish(context.Background(), pool, db.FinishTargetInstallHistoryParams{ID: 17}, func(*db.Queries) (int64, error) {
		t.Fatal("late result touched the target")
		return 0, nil
	})
	if committed || err != nil {
		t.Fatalf("committed=%v err=%v", committed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
