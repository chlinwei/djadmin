package sysconfig

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestConfigUpdateRoutesSchedulerSwitchThroughTransaction(t *testing.T) {
	for _, key := range []string{"sys.scheduler.enabled", "sys.example.value"} {
		t.Run(key, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			now := time.Now().UTC()
			mock.ExpectQuery("FROM sys_config").WithArgs(int64(1)).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "create_time", "update_time", "remark", "key", "value",
					"value_type", "name", "description", "is_readonly", "default_value",
				}).AddRow(1, now, now, nil, key, "true", "bool", "test", nil, false, "true"))
			params := db.UpdateConfigValueParams{
				ID: 1, Value: "false", DefaultValue: sql.NullString{String: "true", Valid: true}, UpdateTime: now,
			}
			transactionError := errors.New("transaction unavailable")
			if key == "sys.scheduler.enabled" {
				// Generic writes, including Reset, must not fall through to a bare UPDATE.
				mock.ExpectBegin().WillReturnError(transactionError)
			} else {
				mock.ExpectExec("UPDATE sys_config").WithArgs(params.Value, params.DefaultValue, now, int64(1)).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			err = NewRepository(pool).Update(context.Background(), params)
			if key == "sys.scheduler.enabled" && !errors.Is(err, transactionError) {
				t.Fatalf("scheduler transaction bypassed: %v", err)
			}
			if key != "sys.scheduler.enabled" && err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
