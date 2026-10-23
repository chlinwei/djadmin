//go:build !postgres

package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

// The opt-in DSN must target a disposable MySQL 8+ server and permit CREATE/DROP
// DATABASE. Never migrate or write its named database: all fixtures live in a
// uniquely named database that this test creates and removes.
func TestDurableSchedulerMySQLSmoke(t *testing.T) {
	dsn := os.Getenv("SCHEDULER_SMOKE_DSN")
	if dsn == "" {
		t.Skip("SCHEDULER_SMOKE_DSN not set; requires disposable MySQL 8+ with CREATE/DROP DATABASE")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid SCHEDULER_SMOKE_DSN")
	}
	config.DBName, config.ParseTime, config.Loc, config.MultiStatements = "", true, time.UTC, false
	admin, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	name := "djadmin_scheduler_smoke_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	smokeExec(t, ctx, admin, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci")
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := admin.ExecContext(cleanupCtx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Errorf("remove isolated smoke database %s: %v", name, err)
		}
	})
	config.DBName = name
	pool, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	pool.SetMaxOpenConns(8)
	schema, err := os.ReadFile(filepath.Join("..", "..", "db", "schema", "mysql", "001_identity_rbac_config.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"sys_menu", "sys_config", "scheduler_scheduledtask", "scheduler_scheduledtasklog", "audit_login_log"} {
		statement := smokeTableDDL(t, string(schema), table)
		if table == "scheduler_scheduledtask" {
			statement = strings.ReplaceAll(statement, "  `schedule_timezone` varchar(64) NOT NULL DEFAULT 'UTC',\n", "")
		}
		smokeExec(t, ctx, pool, statement)
	}
	now := time.Now().UTC()
	smokeExec(t, ctx, pool, `INSERT INTO scheduler_scheduledtask
		(id, create_time, update_time, name, code, enabled, next_run_time, is_running, cron_expression)
		VALUES (1, ?, ?, 'smoke cleanup', 'cleanup_login_audit_logs', TRUE, ?, TRUE, '* * * * *')`,
		now, now, now.Add(-365*24*time.Hour))
	smokeMigration(t, ctx, pool, "up")
	task, err := NewRepository(pool).GetTask(ctx, 1)
	if err != nil || task.NextRunTime.Valid || task.IsRunning || task.ScheduleTimezone != "UTC" {
		t.Fatalf("migration must clear only the untrusted snapshot/running marker: task=%+v err=%v", task, err)
	}
	// Down/up round-trip validates the inverse DDL on an isolated database only.
	smokeMigration(t, ctx, pool, "down")
	smokeMigration(t, ctx, pool, "up")
	smokeExec(t, ctx, pool, "INSERT INTO sys_config (create_time, update_time, `key`, value, value_type, name, is_readonly) VALUES (?, ?, 'sys.scheduler.enabled', 'true', 'bool', 'scheduler', FALSE)", now, now)
	smokeExec(t, ctx, pool, "INSERT INTO sys_config (create_time, update_time, `key`, value, value_type, name, is_readonly) VALUES (?, ?, 'sys.audit.login_logs.retention_days', '90', 'int', 'retention', FALSE)", now, now)
	insertAudit := func() {
		smokeExec(t, ctx, pool, `INSERT INTO audit_login_log (username, status, client_ip, user_agent, message, login_time)
			VALUES ('smoke', 'success', '', '', '', ?)`, now.Add(-365*24*time.Hour))
	}
	repository := NewRepository(pool)
	first, second := NewManager(repository), NewManager(NewRepository(pool))
	if err := first.scanTask(ctx, 1, now); err != nil {
		t.Fatal(err)
	}
	smokeCount(t, ctx, pool, "SELECT COUNT(*) FROM scheduler_execution", 0)
	smokeExec(t, ctx, pool, "UPDATE scheduler_scheduledtask SET next_run_time = ? WHERE id = 1", now.Add(-24*time.Hour))
	runBoth := func(fn func(*Manager) error) {
		t.Helper()
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, manager := range []*Manager{first, second} {
			wg.Add(1)
			go func(manager *Manager) {
				defer wg.Done()
				errs <- fn(manager)
			}(manager)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	runBoth(func(manager *Manager) error { return manager.scanTask(ctx, 1, now) })
	smokeCount(t, ctx, pool, "SELECT COUNT(*) FROM scheduler_execution WHERE status = 'pending'", 1)
	insertAudit()
	runBoth(func(manager *Manager) error { return manager.executeTask(ctx, 1) })
	smokeCount(t, ctx, pool, "SELECT COUNT(*) FROM audit_login_log", 0)
	smokeCount(t, ctx, pool, "SELECT COUNT(*) FROM scheduler_scheduledtasklog", 1)
	smokeCount(t, ctx, pool, "SELECT COUNT(*) FROM scheduler_execution WHERE status = 'succeeded'", 1)

	// Model a process dying after cleanup but before terminal/log commit.
	insertAudit()
	if err := repository.EnqueueManual(ctx, 1); err != nil {
		t.Fatal(err)
	}
	crashed := errors.New("simulated crash before commit")
	err = repository.transaction(ctx, func(tx *Repository) error {
		if _, err := tx.queries.LockSchedulerTask(ctx, 1); err != nil {
			return err
		}
		if _, err := tx.queries.GetPendingSchedulerExecution(ctx, 1); err != nil {
			return err
		}
		if _, err := tx.CleanupLoginAudits(ctx, now.AddDate(0, 0, -90)); err != nil {
			return err
		}
		return crashed
	})
	if !errors.Is(err, crashed) {
		t.Fatal(err)
	}
	smokeCount(t, ctx, pool, "SELECT COUNT(*) FROM audit_login_log", 1)
	smokeCount(t, ctx, pool, "SELECT COUNT(*) FROM scheduler_execution WHERE status = 'pending'", 1)
	if err := NewManager(NewRepository(pool)).executeTask(ctx, 1); err != nil {
		t.Fatal(err)
	}
	smokeCount(t, ctx, pool, "SELECT COUNT(*) FROM audit_login_log", 0)
	smokeCount(t, ctx, pool, "SELECT COUNT(*) FROM scheduler_scheduledtasklog", 2)

	if err := repository.EnqueueManual(ctx, 1); err != nil {
		t.Fatal(err)
	}
	// A held task lock fences a second scheduler and makes edit wait until the
	// holder commits/rolls back, rather than cancelling an executing cleanup.
	lock, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback()
	if _, err := db.New(lock).LockSchedulerTask(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := second.executeTask(ctx, 1); err != nil {
		t.Fatal(err)
	}
	editDone := make(chan error, 1)
	go func() {
		editDone <- repository.SetEnabled(ctx, db.SetScheduledTaskEnabledParams{ID: 1, Enabled: false, UpdateTime: time.Now().UTC()})
	}()
	select {
	case err := <-editDone:
		t.Fatalf("edit bypassed executing task lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := lock.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := <-editDone; err != nil {
		t.Fatal(err)
	}
	smokeCount(t, ctx, pool, "SELECT COUNT(*) FROM scheduler_execution WHERE status = 'pending'", 0)
	taskDTO, err := NewService(repository).GetTask(ctx, 1)
	if err != nil || taskDTO.IsPending || taskDTO.ExecutionStatus == nil || *taskDTO.ExecutionStatus != "cancelled" || taskDTO.NextRunTime != nil {
		t.Fatalf("cancelled task state: %+v err=%v", taskDTO, err)
	}
	if err := repository.SetEnabled(ctx, db.SetScheduledTaskEnabledParams{ID: 1, Enabled: true, UpdateTime: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	task, err = repository.GetTask(ctx, 1)
	if err != nil || !task.NextRunTime.Valid || !task.NextRunTime.Time.After(time.Now().UTC()) {
		t.Fatalf("reenable must start in the future: task=%+v err=%v", task, err)
	}
}

func smokeExec(t *testing.T, ctx context.Context, pool *sql.DB, statement string, args ...any) {
	t.Helper()
	if _, err := pool.ExecContext(ctx, statement, args...); err != nil {
		t.Fatal(err)
	}
}

func smokeCount(t *testing.T, ctx context.Context, pool *sql.DB, statement string, want int64) {
	t.Helper()
	var count int64
	if err := pool.QueryRowContext(ctx, statement).Scan(&count); err != nil || count != want {
		t.Fatalf("%s: count=%d want=%d err=%v", statement, count, want, err)
	}
}

func smokeTableDDL(t *testing.T, schema, table string) string {
	t.Helper()
	start := strings.Index(schema, "CREATE TABLE `"+table+"` (")
	if start < 0 {
		t.Fatalf("missing schema table %s", table)
	}
	end := strings.Index(schema[start:], ";")
	if end < 0 {
		t.Fatal("unterminated DDL")
	}
	return schema[start : start+end+1]
}

func smokeMigration(t *testing.T, ctx context.Context, pool *sql.DB, direction string) {
	t.Helper()
	migration, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "mysql", "000053_durable_scheduler."+direction+".sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(migration), ";") {
		if strings.TrimSpace(statement) != "" {
			smokeExec(t, ctx, pool, statement)
		}
	}
}
