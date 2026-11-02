package logcollect

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"autoadmin/internal/api/response"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestEnabledPatchUsesNarrowWritesAndPreservesCompletionHook(t *testing.T) {
	for _, tt := range []struct{ resource, table string }{
		{"cluster", "monitor_elasticsearch_cluster"},
		{"retention", "monitor_log_retention_tier"},
		{"filter", "monitor_log_collection_filter_rule"},
	} {
		for _, missing := range []bool{false, true} {
			t.Run(tt.resource+map[bool]string{false: "/success", true: "/missing"}[missing], func(t *testing.T) {
				pool, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer pool.Close()
				mock.ExpectBegin()
				lock := mock.ExpectQuery("SELECT enabled FROM " + tt.table + " WHERE id = .* FOR UPDATE").WithArgs(int64(9))
				if missing {
					lock.WillReturnError(sql.ErrNoRows)
					mock.ExpectRollback()
				} else {
					lock.WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(true))
					mock.ExpectExec("UPDATE "+tt.table+" SET enabled = .* update_time = .* WHERE id =").
						WithArgs(false, sqlmock.AnyArg(), int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectCommit()
				}
				h := &Handler{db: pool}
				called := false
				engine := gin.New()
				engine.PATCH("/:id/", func(c *gin.Context) {
					if !h.enabledPatch(c, tt.resource, func(c *gin.Context, id int64) {
						called = true
						response.Success(c, gin.H{"id": id, "enabled": false})
					}) {
						t.Fatal("state-only body did not select narrow update")
					}
				})
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, httptest.NewRequest("PATCH", "/9/", strings.NewReader(`{"enabled":false}`)))
				var envelope response.Envelope
				if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if missing && envelope.Code != 404 || !missing && envelope.Code != 200 || called == missing {
					t.Fatalf("unexpected response or completion hook: %s called=%v", recorder.Body.String(), called)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
