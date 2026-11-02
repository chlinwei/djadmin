package identity

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

func TestOwnBindingStatusIsOwnershipScoped(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		code int
	}{
		{"enable own", `{"enabled":true}`, 200},
		{"disable own", `{"enabled":false}`, 200},
		{"foreign or missing", `{"enabled":true}`, 404},
		{"disabled media", `{"enabled":true}`, 400},
		{"null", `{"enabled":null}`, 400},
		{"string", `{"enabled":"false"}`, 400},
		{"mixed fields", `{"enabled":true,"user_id":8}`, 400},
		{"database failure", `{"enabled":true}`, 600},
		{"write failure", `{"enabled":true}`, 600},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			if tt.name != "null" && tt.name != "string" && tt.name != "mixed fields" {
				mock.ExpectBegin()
				lock := mock.ExpectQuery("WHERE b.id = .* AND b.user_id = .* FOR UPDATE").WithArgs(int64(9), int32(7))
				switch tt.name {
				case "foreign or missing":
					lock.WillReturnError(sql.ErrNoRows)
				case "database failure":
					lock.WillReturnError(sql.ErrConnDone)
				default:
					lock.WillReturnRows(sqlmock.NewRows([]string{"enabled", "media_enabled"}).AddRow(false, tt.name != "disabled media"))
				}
				if tt.code == 200 || tt.name == "write failure" {
					update := mock.ExpectExec("UPDATE monitor_user_alert_media_binding SET enabled = .* update_time = .* WHERE id = .* AND user_id =").
						WithArgs(tt.name != "disable own", sqlmock.AnyArg(), int64(9), int32(7))
					if tt.name == "write failure" {
						update.WillReturnError(sql.ErrConnDone)
					} else {
						update.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
				if tt.code == 200 {
					mock.ExpectCommit()
				} else {
					mock.ExpectRollback()
				}
			}
			handler := NewHandler(&Service{repository: NewRepository(pool)})
			engine := gin.New()
			engine.PATCH("/bindings/:id/", func(c *gin.Context) {
				c.Set(ClaimsContextKey, &Claims{UserID: 7})
				handler.UpdateOwnAlertMediaBindingEnabled(c)
			})
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest("PATCH", "/bindings/9/", strings.NewReader(tt.body)))
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Code != tt.code {
				t.Fatalf("unexpected response: %s", recorder.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
