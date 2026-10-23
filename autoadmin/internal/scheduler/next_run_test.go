package scheduler

import (
	"database/sql"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

func TestDisplayNextRunTimePreservesDurableOverdueCursor(t *testing.T) {
	cursor := "2020-01-01T00:00:00Z"
	task := Task{Enabled: true, Supported: true, EffectiveCronExpression: "* * * * *", NextRunTime: &cursor}
	if next := displayNextRunTime(task); next == nil || *next != cursor {
		t.Fatalf("must display persisted cursor, got %v", next)
	}
	task.Enabled = false
	if displayNextRunTime(task) != nil {
		t.Fatal("disabled task has no next run")
	}
}

func TestNextRunUsesPersistedTimezoneNotProcessLocal(t *testing.T) {
	now := time.Date(2026, 10, 23, 0, 30, 0, 0, time.UTC)
	for _, test := range []struct {
		zone string
		want time.Time
	}{
		{"Asia/Shanghai", time.Date(2026, 10, 23, 1, 0, 0, 0, time.UTC)},
		{"UTC", time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)},
	} {
		next, err := nextRunAt("0 9 * * *", test.zone, now)
		if err != nil || !next.Valid || !next.Time.Equal(test.want) {
			t.Fatalf("%s: next=%v err=%v want=%v", test.zone, next, err, test.want)
		}
	}
}

func TestNextRunRejectsInvalidCronAndLocalTimezone(t *testing.T) {
	for _, expression := range []string{"invalid", "* * * *", "* * * * * *", ""} {
		if _, err := nextRunAt(expression, "UTC", time.Now()); err == nil {
			t.Fatalf("accepted invalid cron %q", expression)
		}
	}
	for _, zone := range []string{"Local", "", "not/a/timezone"} {
		if _, err := nextRunAt("* * * * *", zone, time.Now()); err == nil {
			t.Fatalf("accepted invalid/non-durable timezone %q", zone)
		}
	}
}

func TestTaskDTOExposesPendingUntilAtomicTerminalCommit(t *testing.T) {
	row := db.GetScheduledTaskRow{
		Code: "cleanup_login_audit_logs", Enabled: true, ScheduleTimezone: "Asia/Shanghai",
		CronExpression:  sql.NullString{String: "* * * * *", Valid: true},
		NextRunTime:     sql.NullTime{Time: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		ExecutionID:     sql.NullString{String: "execution", Valid: true},
		ExecutionStatus: sql.NullString{String: "pending", Valid: true},
	}
	task := mapTask(row)
	if !task.IsPending || task.IsRunning || task.ExecutionID == nil || *task.ExecutionID != "execution" || task.ScheduleTimezone != "Asia/Shanghai" || task.NextRunTime == nil {
		t.Fatalf("pending DTO: %+v", task)
	}
	row.ExecutionStatus.String = "cancelled"
	task = mapTask(row)
	if task.IsPending || task.ExecutionStatus == nil || *task.ExecutionStatus != "cancelled" {
		t.Fatalf("cancelled DTO: %+v", task)
	}
}
