package scheduler

import (
	"database/sql"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

type Log struct {
	ID              int64    `json:"id"`
	TaskName        string   `json:"task_name"`
	RunTime         string   `json:"run_time"`
	Status          string   `json:"status"`
	Message         *string  `json:"message"`
	DurationSeconds *float64 `json:"duration_seconds"`
	Output          *string  `json:"output"`
}

func mapListLogs(rows []db.ListScheduledTaskLogsRow) []Log {
	result := make([]Log, 0, len(rows))
	for _, row := range rows {
		result = append(result, Log{
			ID: row.ID, TaskName: row.TaskName, RunTime: formatLogTime(row.RunTime), Status: row.Status,
			Message: stringPtr(row.Message), DurationSeconds: floatPtr(row.DurationSeconds), Output: stringPtr(row.Output),
		})
	}
	return result
}

func mapLog(row db.GetScheduledTaskLogRow) Log {
	return Log{
		ID: row.ID, TaskName: row.TaskName, RunTime: formatLogTime(row.RunTime), Status: row.Status,
		Message: stringPtr(row.Message), DurationSeconds: floatPtr(row.DurationSeconds), Output: stringPtr(row.Output),
	}
}

func floatPtr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func formatLogTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.999999Z")
}
