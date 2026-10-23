package scheduler

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
)

type futureTime struct{}

func (futureTime) Match(value driver.Value) bool {
	stamp, ok := value.(time.Time)
	return ok && stamp.After(time.Now().UTC())
}

var taskColumns = []string{
	"id", "create_time", "update_time", "remark", "name", "code", "description",
	"enabled", "interval_minutes", "last_run_time", "last_status", "last_message",
	"menu_id", "next_run_time", "is_running", "cron_expression", "schedule_timezone",
}

func taskRows(code string, enabled bool, cursor any) *sqlmock.Rows {
	now := time.Now().UTC()
	return sqlmock.NewRows(taskColumns).AddRow(
		int64(1), now, now, nil, "cleanup", code, nil, enabled, nil, nil, nil, nil,
		nil, cursor, false, "* * * * *", "UTC",
	)
}

func pendingRows(trigger string) *sqlmock.Rows {
	now := time.Now().UTC()
	return sqlmock.NewRows([]string{"id", "task_id", "trigger_type", "status", "scheduled_at", "created_at", "finished_at"}).
		AddRow("execution-1", int64(1), trigger, "pending", now, now, nil)
}

func emptyPendingRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "task_id", "trigger_type", "status", "scheduled_at", "created_at", "finished_at"})
}

func expectTaskLock(mock sqlmock.Sqlmock, skip bool, code string, enabled bool, cursor any) {
	query := `(?s)SELECT .* FROM scheduler_scheduledtask WHERE id = \? FOR UPDATE`
	if skip {
		query += ` SKIP LOCKED`
	}
	mock.ExpectQuery(query).WithArgs(int64(1)).WillReturnRows(taskRows(code, enabled, cursor))
}

func expectGlobalLock(mock sqlmock.Sqlmock, enabled string) {
	now := time.Now().UTC()
	mock.ExpectQuery("(?s)SELECT .* FROM sys_config WHERE .key. = 'sys.scheduler.enabled' FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "create_time", "update_time", "remark", "key", "value", "value_type", "name", "description", "is_readonly", "default_value"}).
			AddRow(int64(1), now, now, nil, "sys.scheduler.enabled", enabled, "bool", "", nil, false, "true"))
}

func newManagerMock(t *testing.T) (*Manager, sqlmock.Sqlmock) {
	t.Helper()
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		pool.Close()
	})
	return NewManager(NewRepository(pool)), mock
}

func TestScanInitializesWithoutBackfillingSnapshot(t *testing.T) {
	manager, mock := newManagerMock(t)
	now := time.Date(2026, 10, 23, 0, 30, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectTaskLock(mock, true, "cleanup_login_audit_logs", true, nil)
	expectGlobalLock(mock, "true")
	mock.ExpectExec("UPDATE scheduler_scheduledtask SET next_run_time").
		WithArgs(now.Add(time.Minute), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := manager.scanTask(context.Background(), 1, now); err != nil {
		t.Fatal(err)
	}
}

func TestScanCoalescesMissedRunsAndPersistsFutureCursor(t *testing.T) {
	for _, alreadyPending := range []bool{false, true} {
		t.Run(map[bool]string{false: "enqueue-once", true: "keep-existing"}[alreadyPending], func(t *testing.T) {
			manager, mock := newManagerMock(t)
			now := time.Date(2026, 10, 23, 0, 30, 0, 0, time.UTC)
			overdue := now.Add(-24 * time.Hour)
			mock.ExpectBegin()
			expectTaskLock(mock, true, "cleanup_login_audit_logs", true, overdue)
			expectGlobalLock(mock, "true")
			pending := emptyPendingRows()
			if alreadyPending {
				pending = pendingRows("automatic")
			}
			mock.ExpectQuery("(?s)SELECT .* FROM scheduler_execution").WithArgs(int64(1)).WillReturnRows(pending)
			if !alreadyPending {
				mock.ExpectExec("INSERT INTO scheduler_execution").
					WithArgs(sqlmock.AnyArg(), int64(1), "automatic", overdue, now).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectExec("UPDATE scheduler_scheduledtask SET next_run_time").
				WithArgs(now.Add(time.Minute), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			if err := manager.scanTask(context.Background(), 1, now); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestScanErrorsRollbackInsteadOfAdvancingCursor(t *testing.T) {
	manager, mock := newManagerMock(t)
	now := time.Now().UTC()
	mock.ExpectBegin()
	expectTaskLock(mock, true, "cleanup_login_audit_logs", true, now.Add(-time.Hour))
	expectGlobalLock(mock, "true")
	mock.ExpectQuery("(?s)SELECT .* FROM scheduler_execution").WithArgs(int64(1)).WillReturnError(errors.New("database unavailable"))
	mock.ExpectRollback()
	if err := manager.scanTask(context.Background(), 1, now); err == nil {
		t.Fatal("configuration/database errors must surface")
	}
}

func TestConcurrentInstanceSkipsLockedTask(t *testing.T) {
	manager, mock := newManagerMock(t)
	for _, execute := range []bool{false, true} {
		mock.ExpectBegin()
		mock.ExpectQuery(`(?s)SELECT .* FROM scheduler_scheduledtask WHERE id = \? FOR UPDATE SKIP LOCKED`).
			WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows(taskColumns))
		mock.ExpectCommit()
		var err error
		if execute {
			err = manager.executeTask(context.Background(), 1)
		} else {
			err = manager.scanTask(context.Background(), 1, time.Now().UTC())
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestExecutionWorkersDoNotBlockScheduleScan(t *testing.T) {
	manager, mock := newManagerMock(t)
	mock.MatchExpectationsInOrder(false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	work := make(chan int64)
	var workers sync.WaitGroup
	for _, id := range []int64{1, 3} {
		mock.ExpectBegin()
		mock.ExpectQuery(`(?s)SELECT .* FROM scheduler_scheduledtask WHERE id = \? FOR UPDATE SKIP LOCKED`).
			WithArgs(id).WillDelayFor(time.Second).WillReturnError(errors.New("slow database operation"))
		mock.ExpectRollback()
		workers.Add(1)
		go func() {
			defer workers.Done()
			manager.runExecutions(ctx, work)
		}()
		work <- id
	}
	mock.ExpectQuery("SELECT id FROM scheduler_scheduledtask ORDER BY id").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT .* FROM scheduler_scheduledtask WHERE id = \? FOR UPDATE SKIP LOCKED`).
		WithArgs(int64(2)).WillReturnRows(taskRows("unsupported", false, nil))
	mock.ExpectCommit()
	started := time.Now()
	if err := manager.scan(ctx); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) >= time.Second/2 {
		t.Fatal("schedule scan waited for occupied execution workers")
	}
	close(work)
	workers.Wait()
}

func expectExecutionStart(mock sqlmock.Sqlmock, code string) {
	mock.ExpectBegin()
	expectTaskLock(mock, true, code, true, nil)
	mock.ExpectQuery("(?s)SELECT .* FROM scheduler_execution").WithArgs(int64(1)).WillReturnRows(pendingRows("manual"))
	mock.ExpectExec("SAVEPOINT scheduler_cleanup").WillReturnResult(sqlmock.NewResult(0, 0))
}

func expectTerminal(mock sqlmock.Sqlmock, success bool) {
	status, terminal := "成功", "succeeded"
	if !success {
		status, terminal = "失败", "failed"
	}
	mock.ExpectExec("RELEASE SAVEPOINT scheduler_cleanup").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s)UPDATE scheduler_scheduledtask.*SET is_running = FALSE").
		WithArgs(status, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO scheduler_scheduledtasklog").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE scheduler_execution SET status").
		WithArgs(terminal, sqlmock.AnyArg(), "execution-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func TestExecutionCleanupAndTerminalLogShareTransaction(t *testing.T) {
	manager, mock := newManagerMock(t)
	expectExecutionStart(mock, "cleanup_login_audit_logs")
	expectRetentionDays(t, mock, "sys.audit.login_logs.retention_days", "90")
	mock.ExpectExec("DELETE FROM audit_login_log").WillReturnResult(sqlmock.NewResult(0, 5))
	expectTerminal(mock, true)
	if err := manager.executeTask(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}

func TestPartialCleanupFailureRollsBackBeforeDurableFailureLog(t *testing.T) {
	manager, mock := newManagerMock(t)
	expectExecutionStart(mock, "cleanup_ansible_execution_logs")
	expectRetentionDays(t, mock, "sys.automation.logs.retention_days", "30")
	mock.ExpectExec("DELETE FROM automation_execution_host_log").WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectExec("DELETE FROM automation_execution_job_log").WillReturnError(errors.New("delete failed"))
	mock.ExpectExec("ROLLBACK TO SAVEPOINT scheduler_cleanup").WillReturnResult(sqlmock.NewResult(0, 0))
	expectTerminal(mock, false)
	if err := manager.executeTask(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalWriteFailureRollsBackForSafeRestartRecovery(t *testing.T) {
	manager, mock := newManagerMock(t)
	expectExecutionStart(mock, "cleanup_login_audit_logs")
	expectRetentionDays(t, mock, "sys.audit.login_logs.retention_days", "90")
	mock.ExpectExec("DELETE FROM audit_login_log").WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectExec("RELEASE SAVEPOINT scheduler_cleanup").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("(?s)UPDATE scheduler_scheduledtask.*SET is_running = FALSE").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO scheduler_scheduledtasklog").WillReturnError(errors.New("log failed"))
	mock.ExpectRollback()
	if err := manager.executeTask(context.Background(), 1); err == nil {
		t.Fatal("terminal/log failure must roll cleanup back to pending")
	}
	// A fresh process can execute that still-pending record without any lease.
	expectExecutionStart(mock, "cleanup_login_audit_logs")
	expectRetentionDays(t, mock, "sys.audit.login_logs.retention_days", "90")
	mock.ExpectExec("DELETE FROM audit_login_log").WillReturnResult(sqlmock.NewResult(0, 5))
	expectTerminal(mock, true)
	if err := NewManager(manager.repository).executeTask(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}

func TestManualEnqueueDurableAndDuplicateRejected(t *testing.T) {
	manager, mock := newManagerMock(t)
	mock.ExpectBegin()
	expectTaskLock(mock, true, "cleanup_login_audit_logs", true, nil)
	mock.ExpectQuery("(?s)SELECT .* FROM scheduler_execution").WithArgs(int64(1)).WillReturnRows(emptyPendingRows())
	mock.ExpectExec("INSERT INTO scheduler_execution").
		WithArgs(sqlmock.AnyArg(), int64(1), "manual", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := manager.repository.EnqueueManual(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	expectTaskLock(mock, true, "cleanup_login_audit_logs", true, nil)
	mock.ExpectQuery("(?s)SELECT .* FROM scheduler_execution").WithArgs(int64(1)).WillReturnRows(pendingRows("manual"))
	mock.ExpectRollback()
	if err := manager.repository.EnqueueManual(context.Background(), 1); err != ErrTaskRunning {
		t.Fatalf("duplicate enqueue: %v", err)
	}
}

func TestDisableCancelsPendingAndClearsCursorAtomically(t *testing.T) {
	manager, mock := newManagerMock(t)
	mock.ExpectBegin()
	expectTaskLock(mock, false, "cleanup_login_audit_logs", true, time.Now().UTC())
	mock.ExpectExec("UPDATE scheduler_execution SET status = 'cancelled'").
		WithArgs(sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectGlobalLock(mock, "true")
	mock.ExpectExec("UPDATE scheduler_scheduledtask").
		WithArgs(false, nil, sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := manager.repository.SetEnabled(context.Background(), db.SetScheduledTaskEnabledParams{ID: 1, Enabled: false, UpdateTime: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
}

func TestCronEditCancelsOldPendingAndResetsCursor(t *testing.T) {
	manager, mock := newManagerMock(t)
	mock.ExpectBegin()
	expectTaskLock(mock, false, "cleanup_login_audit_logs", true, time.Now().Add(-time.Hour))
	mock.ExpectExec("UPDATE scheduler_execution SET status = 'cancelled'").WillReturnResult(sqlmock.NewResult(0, 1))
	expectGlobalLock(mock, "true")
	mock.ExpectExec("UPDATE scheduler_scheduledtask").
		WithArgs("cleanup", "cleanup_login_audit_logs", nil, true, "*/5 * * * *", futureTime{}, sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := manager.repository.UpdateTask(context.Background(), db.UpdateScheduledTaskParams{
		ID: 1, Name: "cleanup", Code: "cleanup_login_audit_logs", Enabled: true,
		CronExpression: sql.NullString{String: "*/5 * * * *", Valid: true}, UpdateTime: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestGlobalReenableStartsFutureWithoutDisabledCatchup(t *testing.T) {
	manager, mock := newManagerMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM scheduler_scheduledtask ORDER BY id").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	expectTaskLock(mock, false, "cleanup_login_audit_logs", true, time.Now().Add(-24*time.Hour))
	expectGlobalLock(mock, "false")
	mock.ExpectExec("(?s)UPDATE scheduler_execution SET status = 'cancelled'.*trigger_type = 'automatic'").
		WithArgs(sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE scheduler_scheduledtask SET next_run_time").
		WithArgs(futureTime{}, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE sys_config").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := manager.repository.SetSchedulerEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
}

func TestGenericSchedulerConfigUpdatePreservesDefaultAtomically(t *testing.T) {
	for _, previous := range []string{"true", "false"} {
		t.Run(previous, func(t *testing.T) {
			manager, mock := newManagerMock(t)
			now := time.Now().UTC()
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT id FROM scheduler_scheduledtask ORDER BY id").
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
			expectTaskLock(mock, false, "cleanup_login_audit_logs", true, now)
			expectGlobalLock(mock, previous)
			if previous == "true" {
				mock.ExpectExec("(?s)UPDATE scheduler_execution SET status = 'cancelled'.*trigger_type = 'automatic'").
					WithArgs(sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE scheduler_scheduledtask SET next_run_time").
					WithArgs(nil, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectExec("UPDATE sys_config").
				WithArgs("false", "false", now, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			if err := manager.repository.UpdateSchedulerConfig(context.Background(), db.UpdateConfigValueParams{
				ID: 1, Value: "false", DefaultValue: sql.NullString{String: "false", Valid: true}, UpdateTime: now,
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGenericSchedulerConfigRejectsWrongIDAndInvalidValue(t *testing.T) {
	manager, mock := newManagerMock(t)
	if err := manager.repository.UpdateSchedulerConfig(context.Background(), db.UpdateConfigValueParams{Value: "invalid"}); err == nil {
		t.Fatal("invalid switch value accepted")
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM scheduler_scheduledtask ORDER BY id").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	expectGlobalLock(mock, "true")
	mock.ExpectRollback()
	if err := manager.repository.UpdateSchedulerConfig(context.Background(), db.UpdateConfigValueParams{ID: 99, Value: "false"}); err == nil {
		t.Fatal("wrong config ID accepted")
	}
}

func TestManagerShutdownCancelsAndWaitsForRun(t *testing.T) {
	manager, _ := newManagerMock(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDurableSchemaAndCutoverMigrationMatchBothDialects(t *testing.T) {
	for _, dialect := range []string{"mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			root := filepath.Join("..", "..", "db")
			schema, err := os.ReadFile(filepath.Join(root, "schema", dialect, "001_identity_rbac_config.sql"))
			if err != nil {
				t.Fatal(err)
			}
			up, err := os.ReadFile(filepath.Join(root, "migrations", dialect, "000053_durable_scheduler.up.sql"))
			if err != nil {
				t.Fatal(err)
			}
			marker := "CREATE TABLE scheduler_execution ("
			schemaStart, migrationStart := strings.Index(string(schema), marker), strings.Index(string(up), marker)
			if schemaStart < 0 || migrationStart < 0 {
				t.Fatal("missing durable execution table")
			}
			migrationDDL := strings.TrimSpace(string(up)[migrationStart:])
			if !strings.HasPrefix(string(schema)[schemaStart:], migrationDDL) {
				t.Fatal("schema and migration execution DDL diverged")
			}
			if !strings.Contains(string(up), "SET next_run_time = NULL, is_running = FALSE") {
				t.Fatal("cutover must reset untrusted historical cursor and running flag")
			}
			if !strings.Contains(string(up), "schedule_timezone varchar(64) NOT NULL DEFAULT 'UTC'") {
				t.Fatal("cutover must pin an explicit shared timezone")
			}
			down, err := os.ReadFile(filepath.Join(root, "migrations", dialect, "000053_durable_scheduler.down.sql"))
			if err != nil || !strings.Contains(string(down), "DROP TABLE scheduler_execution") || !strings.Contains(string(down), "DROP COLUMN schedule_timezone") {
				t.Fatalf("incomplete inverse migration: %s err=%v", down, err)
			}
		})
	}
}
