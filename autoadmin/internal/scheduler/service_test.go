package scheduler

import "testing"

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
