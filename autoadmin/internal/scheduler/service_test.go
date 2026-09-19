package scheduler

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNextRun(t *testing.T) {
	next, err := nextRun("*/5 * * * *", true)
	if err != nil {
		t.Fatalf("nextRun() error = %v", err)
	}
	if !next.Valid || next.Time.IsZero() {
		t.Fatalf("expected next run, got %+v", next)
	}
}

func TestNextRunRejectsInvalidCron(t *testing.T) {
	if _, err := nextRun("invalid", true); err == nil {
		t.Fatal("expected invalid cron to fail")
	}
}

func TestNextRunDisabledTask(t *testing.T) {
	next, err := nextRun("*/5 * * * *", false)
	if err != nil || next.Valid {
		t.Fatalf("disabled task next run = %+v, error = %v", next, err)
	}
}

// 回归（2026-09-19 现场）：在"定时任务"页点「立即执行」，历史任务只会回一句
// `任务提交失败: 任务 handler 尚未迁移到 Go`——不知道是谁的问题、要不要等、有没有替代。
// 现在 ① 列表/详情带 supported 标记（页面据此把按钮置灰）；② 报错写明任务与"已实现的任务"。
func TestUnsupportedTaskNoteExplainsWhatStillWorks(t *testing.T) {
	note := UnsupportedTaskNote("历史告警对账", "reconcile_prometheus_alert_history")
	for _, want := range []string{"历史告警对账", "reconcile_prometheus_alert_history", "定时调度会跳过它", "登录日志清理", "操作日志清理"} {
		if !strings.Contains(note, want) {
			t.Fatalf("说明里应包含 %q，得到 %q", want, note)
		}
	}
}

// 已实现的任务必须被判为 supported，且**不**带说明（页面据此保持按钮可点）。
func TestSupportedTaskCodesCoverTheCleanupHandlers(t *testing.T) {
	for code, name := range supportedTaskCodes {
		if !IsSupportedTaskCode(code) {
			t.Fatalf("%s (%s) 已注册 handler，必须被判为 supported", code, name)
		}
	}
	if IsSupportedTaskCode("reconcile_prometheus_alert_history") {
		t.Fatal("历史任务不得被判为 supported：判定一旦放宽，页面会放行一个必然失败的执行")
	}
	// 名字非空且排序稳定：它会出现在提示文案与文档里（顺序变了文案就跟着抖）。
	names := SupportedTaskNames()
	if len(names) != len(supportedTaskCodes) || !sort.StringsAreSorted(names) {
		t.Fatalf("SupportedTaskNames() = %v（要求：数量一致且有序）", names)
	}
	for _, want := range []string{"登录日志清理", "操作日志清理"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("SupportedTaskNames() 少了 %q：%v", want, names)
		}
	}
}

// handler 注册表与 supportedTaskCodes 必须一一对应：只登记不实现（或反过来）都会让"可执行"这个
// 结论说谎——前者点了执行才发现没实现，后者明明能跑却被置灰。
func TestWorkerHandlersMatchSupportedCodes(t *testing.T) {
	worker := NewWorker(nil)
	for code := range supportedTaskCodes {
		if _, ok := worker.handlers[code]; !ok {
			t.Errorf("supportedTaskCodes 登记了 %s，但 worker 没有对应 handler", code)
		}
	}
	for code := range worker.handlers {
		if !IsSupportedTaskCode(code) {
			t.Errorf("worker 有 %s 的 handler，却没登记进 supportedTaskCodes（页面会误置灰）", code)
		}
	}
}

// ---- 四类保留期清理（2026-09-19 从历史任务迁到 Go）----
//
// 每个 handler 只做三件事：读 sys_config 的**同名键** → 按保留天数算 cutoff → 删早于 cutoff 的行。
// 这里用 sqlmock 钉住三件事：键名拼写（写错就会静默用默认值，用户配的天数不生效）、
// 删除语句确实发了、以及输出里带上"清了多少 + 保留多少天"（执行日志是清理唯一的证据）。

// expectRetentionDays 让 GetConfigByKey 返回一个保留天数配置。
func expectRetentionDays(t *testing.T, mock sqlmock.Sqlmock, key, days string) {
	t.Helper()
	// 注意 WHERE 里的列名是**反引号包的** `key`（生成物照 schema 原样），
	// 且 (?s) 让 . 能跨行——生成物的 SQL 是多行的。
	mock.ExpectQuery("(?s)SELECT .* FROM sys_config WHERE .key. = \\?").
		WithArgs(key).
		WillReturnRows(sqlmock.NewRows([]string{"id", "create_time", "update_time", "remark", "key", "value", "value_type", "name", "description", "is_readonly", "default_value"}).
			AddRow(int64(1), time.Now().UTC(), time.Now().UTC(), "", key, days, "int", "", "", false, days))
}

func newCleanupWorker(t *testing.T) (*Worker, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	return NewWorker(NewRepository(database)), mock, database
}

func TestCleanupWebSSHSessionLogsUsesConfiguredRetention(t *testing.T) {
	worker, mock, database := newCleanupWorker(t)
	defer database.Close()
	expectRetentionDays(t, mock, "sys.audit.webssh.retention_days", "30")
	mock.ExpectExec(`DELETE FROM assets_webssh_session_log WHERE start_time <`).
		WillReturnResult(sqlmock.NewResult(0, 7))

	output, err := worker.cleanupWebSSHSessionLogs(context.Background())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if !strings.Contains(output, "7") || !strings.Contains(output, "30 days") {
		t.Fatalf("输出要写清清了多少、保留多少天，得到 %q", output)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCleanupAutomationExecutionLogsDeletesChildrenBeforeJobs(t *testing.T) {
	worker, mock, database := newCleanupWorker(t)
	defer database.Close()
	expectRetentionDays(t, mock, "sys.automation.logs.retention_days", "3")
	// 顺序：主机明细 → 字节块 → 作业（外键无级联，顺序反了会撞外键）。
	mock.ExpectExec(`DELETE FROM automation_execution_host_log`).WillReturnResult(sqlmock.NewResult(0, 11))
	mock.ExpectExec(`DELETE FROM automation_execution_job_log`).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`DELETE FROM automation_execution_job WHERE end_time IS NOT NULL`).WillReturnResult(sqlmock.NewResult(0, 4))

	output, err := worker.cleanupAutomationExecutionLogs(context.Background())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	for _, want := range []string{"4 automation jobs", "11 host results", "2 log chunks", "3 days"} {
		if !strings.Contains(output, want) {
			t.Fatalf("输出里应包含 %q，得到 %q", want, output)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// 运行中的作业绝不能被清：SQL 里必须有 end_time IS NOT NULL（只有已结束的作业才有 end_time）。
func TestAutomationCleanupOnlyTouchesFinishedJobs(t *testing.T) {
	for _, name := range []string{"DeleteAutomationExecutionHostLogsBefore", "DeleteAutomationExecutionJobLogsBefore", "DeleteAutomationExecutionJobsBefore"} {
		statement := readSchedulerQuery(t, name)
		if !strings.Contains(statement, "end_time IS NOT NULL") {
			t.Fatalf("%s 少了 end_time IS NOT NULL：运行中的作业会被删掉\n%s", name, statement)
		}
	}
}

func TestCleanupMonitorInstallHistoriesKeepsLatestPerTarget(t *testing.T) {
	worker, mock, database := newCleanupWorker(t)
	defer database.Close()
	expectRetentionDays(t, mock, "sys.monitor.install_history.retention_days", "180")
	mock.ExpectExec(`DELETE FROM monitor_target_install_history AS hist`).WillReturnResult(sqlmock.NewResult(0, 5))

	output, err := worker.cleanupMonitorInstallHistories(context.Background())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if !strings.Contains(output, "5") || !strings.Contains(output, "180 days") {
		t.Fatalf("输出 = %q", output)
	}
	// "每个目标至少保留最新一条"是这条配置的说明，SQL 里必须真的这么做。
	statement := readSchedulerQuery(t, "DeleteMonitorInstallHistoriesBefore")
	if !strings.Contains(statement, "MAX(latest.id)") || !strings.Contains(statement, "GROUP BY latest.target_id") {
		t.Fatalf("保留最新一条的逻辑没了：\n%s", statement)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCleanupAlertHistoriesOnlyDeletesResolved(t *testing.T) {
	worker, mock, database := newCleanupWorker(t)
	defer database.Close()
	expectRetentionDays(t, mock, "sys.monitor.alert_history.retention_days", "7")
	// 顺序：投递记录 → 通知事件 → 告警行（外键无级联，反了就撞外键——真库上实测过 1451）。
	mock.ExpectExec(`DELETE FROM monitor_alert_notification_delivery`).WillReturnResult(sqlmock.NewResult(0, 9))
	mock.ExpectExec(`DELETE FROM monitor_alert_notification_event`).WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectExec(`DELETE FROM monitor_alert_history`).WillReturnResult(sqlmock.NewResult(0, 3))

	output, err := worker.cleanupAlertHistories(context.Background())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	for _, want := range []string{"3 resolved alert rows", "5 notification events", "9 deliveries", "7 days"} {
		if !strings.Contains(output, want) {
			t.Fatalf("输出里应包含 %q，得到 %q", want, output)
		}
	}
	// 仍在 firing 的告警是"当前状态"，删了会让告警页凭空少一条正在发的告警。
	for _, name := range []string{"DeleteResolvedAlertHistoriesBefore", "DeleteAlertNotificationEventsForHistoriesBefore", "DeleteAlertNotificationDeliveriesForHistoriesBefore"} {
		statement := readSchedulerQuery(t, name)
		if !strings.Contains(statement, "state = 'resolved'") {
			t.Fatalf("%s 必须只清已恢复的记录：\n%s", name, statement)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// 键名写错时退回默认值（而不是清零或崩掉）：保留期读不到就按默认值清理，绝不能因为读不到配置
// 就"不清理"（表会一直涨）或"全清"（数据没了）。
func TestRetentionDaysFallsBackWhenConfigMissing(t *testing.T) {
	worker, mock, database := newCleanupWorker(t)
	defer database.Close()
	// 注意 WHERE 里的列名是**反引号包的** `key`（生成物照 schema 原样），
	// 且 (?s) 让 . 能跨行——生成物的 SQL 是多行的。
	mock.ExpectQuery("(?s)SELECT .* FROM sys_config WHERE .key. = \\?").
		WithArgs("sys.audit.webssh.retention_days").
		WillReturnRows(sqlmock.NewRows([]string{"id", "create_time", "update_time", "remark", "key", "value", "value_type", "name", "description", "is_readonly", "default_value"}))
	mock.ExpectExec(`DELETE FROM assets_webssh_session_log`).WillReturnResult(sqlmock.NewResult(0, 0))

	output, err := worker.cleanupWebSSHSessionLogs(context.Background())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if !strings.Contains(output, "30 days") {
		t.Fatalf("读不到配置时要用默认 30 天，得到 %q", output)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// readSchedulerQuery 从 db/queries/mysql/scheduler.sql 里取一条语句的 SQL 文本（只含 SQL 行）。
func readSchedulerQuery(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "queries", "mysql", "scheduler.sql"))
	if err != nil {
		t.Fatalf("read scheduler.sql: %v", err)
	}
	content := string(raw)
	marker := "-- name: " + name
	start := strings.Index(content, marker)
	if start < 0 {
		t.Fatalf("找不到语句 %s", name)
	}
	rest := content[start+len(marker):]
	if next := strings.Index(rest, "-- name: "); next >= 0 {
		rest = rest[:next]
	}
	lines := []string{}
	for _, line := range strings.Split(rest, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
