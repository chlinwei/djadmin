package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestWebSSHSessionListPreservesCurrentStatus(t *testing.T) {
	for _, status := range []string{"connected", "closed", "failed"} {
		t.Run(status, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM assets_webssh_session_log").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery("SELECT s.id, s.host_id AS host").WillReturnRows(sqlmock.NewRows([]string{
				"id", "host", "host_name", "host_ip", "user_id", "username", "client_ip", "user_agent",
				"status", "start_time", "end_time", "duration_seconds", "close_code", "error_message",
				"input_bytes", "command_count", "recorded_content_bytes", "is_content_truncated",
			}).AddRow(123, 42, "host", "10.0.0.1", 7, "operator", "10.0.0.9", "test",
				status, time.Now().UTC(), nil, nil, nil, "", 6, 1, 12, false))
			filter := WebSSHFilter{Status: status}
			if params := webSSHParams(filter); !params.ExactStatus.Valid || params.ExactStatus.String != status {
				t.Fatalf("status filter was rewritten: %+v", params)
			}
			rows, count, err := NewRepository(pool).ListWebSSHSessions(context.Background(), filter, pagination.New(1, 10))
			if err != nil || count != 1 || len(rows) != 1 || rows[0].Status != status {
				t.Fatalf("live session status was rewritten: %+v, %d, %v", rows, count, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOperationLogJSON(t *testing.T) {
	for _, tc := range []struct {
		name     string
		duration sql.NullInt32
		userID   sql.NullInt32
		wantMS   string
		wantUser string
	}{
		{"measured", sql.NullInt32{Int32: 128, Valid: true}, sql.NullInt32{Int32: 7, Valid: true}, "128", "7"},
		{"zero", sql.NullInt32{Valid: true}, sql.NullInt32{Valid: true}, "0", "0"},
		{"missing", sql.NullInt32{Int32: 128}, sql.NullInt32{Int32: 7}, "null", "null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created := time.Date(2026, 10, 29, 12, 20, 32, 0, time.FixedZone("UTC+8", 8*60*60))
			row := db.AuditOperationLog{
				ID: 72251, Username: "operator", UserID: tc.userID,
				Method: "POST", Path: "/test/", RouteName: "/test/",
				ClientIp: "127.0.0.1", UserAgent: "test", StatusCode: 200,
				DurationMs: tc.duration, Message: "success", CreatedAt: created,
				RequestData: "{}", ResponseData: `{"code":200}`,
			}
			data, err := json.Marshal(operationLog(row))
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			if string(fields["duration_ms"]) != tc.wantMS || string(fields["user_id"]) != tc.wantUser {
				t.Fatalf("nullable fields must be JSON numbers or null: %s", data)
			}
			if string(fields["created_at"]) != `"2026-10-29T04:20:32Z"` {
				t.Fatalf("created_at must be UTC: %s", data)
			}
			if len(fields) != 14 || string(fields["id"]) != "72251" ||
				string(fields["status_code"]) != "200" || string(fields["message"]) != `"success"` ||
				string(fields["request_data"]) != `"{}"` || string(fields["response_data"]) != `"{\"code\":200}"` {
				t.Fatalf("operation log fields changed: %s", data)
			}
		})
	}
}
